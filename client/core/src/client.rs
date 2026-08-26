//! The `VpnClient` façade: the single object each platform's UI drives. It owns
//! the session, the connection state machine, and the orchestration of
//! enroll → connect → disconnect, delegating network I/O to a `Transport` and
//! the OS tunnel to a `TunnelProvider`.

use std::net::{SocketAddr, TcpStream};
use std::sync::Mutex;
use std::time::{Duration, Instant};

use crate::api::ApiClient;
use crate::error::{CoreError, Result};
use crate::keys::{self, KeyPair};
use crate::model::{ConnectRequest, DeviceRegistration, EnrollRequest, Location, Server, StatsReport};
use crate::state::ConnectionState;
use crate::transport::Transport;
use crate::tunnel::{self, TunnelConfig, TunnelProvider};

/// Session state the platform persists (private key in the OS keystore, refresh
/// token in secure storage) and restores on next launch.
#[derive(Debug, Clone)]
pub struct Session {
    pub device_id: String,
    pub user_id: String,
    pub refresh_token: String,
    pub private_key: String,
    pub public_key: String,
}

/// A compact summary of the active connection for the UI.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ConnectionSummary {
    pub connection_id: String,
    pub server_name: String,
    pub country: String,
    pub city: String,
    pub endpoint: String,
    pub assigned_ip: String,
    pub expires_at: String,
}

struct Inner {
    access_token: String,
    session: Option<Session>,
    state: ConnectionState,
    active_connection_id: Option<String>,
    current_server: Option<Server>,

    // Live statistics, computed from the platform's tunnel byte counters.
    connected_at: Option<Instant>,
    last_sample: Option<(u64, u64, Instant)>, // (rx_bytes, tx_bytes, at)
    download_bps: u64,                          // bytes/sec
    upload_bps: u64,                            // bytes/sec
    latency_ms: u32,                            // 0 = unknown
    probe_addr: Option<String>,                 // gateway in-tunnel addr for latency
}

/// Live connection statistics for the UI. Rates are bytes per second; duration
/// is seconds since the tunnel came up. Values are real measurements fed from
/// the OS tunnel — never simulated.
#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct ConnectionStats {
    pub download_bps: u64,
    pub upload_bps: u64,
    pub latency_ms: u32,
    pub duration_seconds: u64,
}

/// Computes a byte-rate from cumulative counters. Guards counter resets.
fn rate(prev: u64, cur: u64, dt_secs: f64) -> u64 {
    if dt_secs <= 0.0 || cur < prev {
        return 0;
    }
    ((cur - prev) as f64 / dt_secs) as u64
}

pub struct VpnClient {
    api: ApiClient,
    provider: Box<dyn TunnelProvider>,
    inner: Mutex<Inner>,
}

impl VpnClient {
    /// Creates an unauthenticated client. `transport` performs HTTP; `provider`
    /// is the native tunnel layer.
    pub fn new(base_url: impl Into<String>, transport: Box<dyn Transport>, provider: Box<dyn TunnelProvider>) -> Self {
        Self {
            api: ApiClient::new(base_url, transport),
            provider,
            inner: Mutex::new(Inner {
                access_token: String::new(),
                session: None,
                state: ConnectionState::Disconnected,
                active_connection_id: None,
                current_server: None,
                connected_at: None,
                last_sample: None,
                download_bps: 0,
                upload_bps: 0,
                latency_ms: 0,
                probe_addr: None,
            }),
        }
    }

    /// Restores a persisted session (from a previous enrollment). The access
    /// token is obtained lazily via refresh on the next authenticated call.
    pub fn restore(&self, session: Session) {
        let mut inner = self.inner.lock().unwrap();
        inner.session = Some(session);
        inner.access_token.clear();
    }

    /// Enrolls with an invite, generating the device keypair. Returns the
    /// session for the platform to persist. Only the public key is transmitted.
    pub fn enroll(
        &self,
        invite_code: &str,
        email: &str,
        display_name: Option<String>,
        device_name: &str,
        platform: &str,
    ) -> Result<Session> {
        let keypair = keys::generate();
        let req = EnrollRequest {
            invite_code: invite_code.to_string(),
            email: email.to_string(),
            display_name,
            device: DeviceRegistration {
                name: device_name.to_string(),
                platform: platform.to_string(),
                public_key: keypair.public_key.clone(),
            },
        };
        let tokens = self.api.enroll(&req)?;
        let session = Session {
            device_id: tokens.device_id,
            user_id: tokens.user_id,
            refresh_token: tokens.refresh_token,
            private_key: keypair.private_key,
            public_key: keypair.public_key,
        };
        let mut inner = self.inner.lock().unwrap();
        inner.access_token = tokens.access_token;
        inner.session = Some(session.clone());
        Ok(session)
    }

    /// The current connection state.
    pub fn state(&self) -> ConnectionState {
        self.inner.lock().unwrap().state.clone()
    }

    /// The server the client is connected to, if any.
    pub fn current_server(&self) -> Option<Server> {
        self.inner.lock().unwrap().current_server.clone()
    }

    /// Lists selectable countries.
    pub fn locations(&self) -> Result<Vec<Location>> {
        self.authed(|access| self.api.locations(access))
    }

    /// Selects a gateway and leases an address, returning the summary and the
    /// `TunnelConfig` the native layer must use to bring up the OS tunnel. State
    /// moves to `Connecting`; the platform calls [`mark_connected`] once the
    /// native tunnel is actually up, or [`mark_failed`] / [`disconnect`] on
    /// failure. This is the entry point the Apple/Android/Windows apps use: the
    /// core never touches the OS network stack.
    pub fn prepare_connection(&self, country_code: &str) -> Result<(ConnectionSummary, TunnelConfig)> {
        let (device_id, keypair) = {
            let inner = self.inner.lock().unwrap();
            if !inner.state.can_transition_to(&ConnectionState::Connecting) {
                return Err(CoreError::InvalidTransition {
                    from: inner.state.to_string(),
                    to: "connecting".into(),
                });
            }
            let s = inner.session.as_ref().ok_or(CoreError::NotAuthenticated)?;
            (s.device_id.clone(), KeyPair { private_key: s.private_key.clone(), public_key: s.public_key.clone() })
        };
        self.set_state(ConnectionState::Connecting);

        let req = ConnectRequest { country_code: country_code.to_string(), device_id };
        let conn = match self.authed(|access| self.api.connect(access, &req)) {
            Ok(c) => c,
            Err(e) => {
                self.fail(&e);
                return Err(e);
            }
        };

        let config = tunnel::build_config(&keypair, &conn);
        let summary = ConnectionSummary {
            connection_id: conn.connection_id.clone(),
            server_name: conn.server.name.clone(),
            country: conn.server.country.clone(),
            city: conn.server.city.clone(),
            endpoint: conn.server.endpoint.clone(),
            assigned_ip: conn.assigned_ip.clone(),
            expires_at: conn.expires_at.clone(),
        };
        {
            let mut inner = self.inner.lock().unwrap();
            inner.active_connection_id = Some(conn.connection_id);
            inner.current_server = Some(conn.server);
            inner.probe_addr = conn.probe_addr;
        }
        Ok((summary, config))
    }

    /// Marks the tunnel fully established (called by the platform once its native
    /// VPN provider reports the tunnel is up). Resets the statistics baseline.
    pub fn mark_connected(&self) {
        let mut inner = self.inner.lock().unwrap();
        if inner.state.can_transition_to(&ConnectionState::Connected) {
            inner.state = ConnectionState::Connected;
            inner.connected_at = Some(Instant::now());
            inner.last_sample = None;
            inner.download_bps = 0;
            inner.upload_bps = 0;
            inner.latency_ms = 0;
        }
    }

    /// Marks the connection failed (called by the platform if the native tunnel
    /// could not be established or terminated unexpectedly).
    pub fn mark_failed(&self, reason: &str) {
        self.inner.lock().unwrap().state = ConnectionState::Failed(reason.to_string());
    }

    /// Convenience for callers that own a [`TunnelProvider`] (Rust tests, and
    /// platforms wiring the provider through FFI): prepare, bring the provider
    /// up, and mark connected in one call.
    pub fn connect(&self, country_code: &str) -> Result<ConnectionSummary> {
        let (summary, config) = self.prepare_connection(country_code)?;
        if let Err(e) = self.provider.up(&config) {
            self.fail(&e);
            return Err(e);
        }
        self.mark_connected();
        Ok(summary)
    }

    /// Disconnects: tears down the native tunnel and releases the lease.
    pub fn disconnect(&self) -> Result<()> {
        let connection_id = {
            let inner = self.inner.lock().unwrap();
            match &inner.active_connection_id {
                Some(id) => id.clone(),
                None => return Ok(()), // already disconnected
            }
        };
        self.set_state(ConnectionState::Disconnecting);
        self.provider.down()?;
        let _ = self.authed(|access| self.api.disconnect(access, &connection_id));
        let mut inner = self.inner.lock().unwrap();
        inner.state = ConnectionState::Disconnected;
        inner.active_connection_id = None;
        inner.current_server = None;
        inner.connected_at = None;
        inner.last_sample = None;
        inner.download_bps = 0;
        inner.upload_bps = 0;
        inner.latency_ms = 0;
        inner.probe_addr = None;
        Ok(())
    }

    /// Measures round-trip latency THROUGH the tunnel by timing a TCP handshake
    /// to the gateway's in-tunnel probe responder (run by the node agent). This
    /// is a real network measurement — WireGuard itself provides no RTT. Returns
    /// None if there is no probe address or the probe fails.
    fn measure_latency(&self) -> Option<u32> {
        let addr = self.inner.lock().unwrap().probe_addr.clone()?;
        let sock: SocketAddr = addr.parse().ok()?;
        let start = Instant::now();
        match TcpStream::connect_timeout(&sock, Duration::from_millis(1500)) {
            Ok(_) => Some(start.elapsed().as_millis() as u32),
            Err(_) => None,
        }
    }

    /// Feeds the OS tunnel's cumulative byte counters (and optionally a measured
    /// latency) so the core can compute real download/upload rates, and renews
    /// the lease (reporting the rates to the control plane). Call periodically
    /// while connected. Returns the freshly computed stats.
    pub fn report_tunnel_stats(&self, rx_bytes: u64, tx_bytes: u64, latency_ms: Option<u32>) -> Result<ConnectionStats> {
        // Only meaningful while connected.
        if !self.inner.lock().unwrap().state.is_connected() {
            return Ok(ConnectionStats::default());
        }
        // Measure real latency through the tunnel if the platform didn't supply
        // one (done outside the lock — it performs a network round-trip).
        let measured = match latency_ms {
            Some(l) => Some(l),
            None => self.measure_latency(),
        };

        let (connection_id, report) = {
            let mut inner = self.inner.lock().unwrap();
            if !inner.state.is_connected() {
                return Ok(ConnectionStats::default());
            }
            let now = Instant::now();
            if let Some((prx, ptx, at)) = inner.last_sample {
                let dt = now.duration_since(at).as_secs_f64();
                inner.download_bps = rate(prx, rx_bytes, dt);
                inner.upload_bps = rate(ptx, tx_bytes, dt);
            }
            inner.last_sample = Some((rx_bytes, tx_bytes, now));
            if let Some(l) = measured {
                inner.latency_ms = l;
            }
            let report = StatsReport {
                rx_bps: (inner.download_bps as i64) * 8, // report bits/sec upstream
                tx_bps: (inner.upload_bps as i64) * 8,
                latency_ms: inner.latency_ms as i64,
            };
            (inner.active_connection_id.clone(), report)
        };

        if let Some(id) = connection_id {
            // Renew the lease and report stats; ignore transient failures here so
            // stats keep flowing to the UI even if a single renew hiccups.
            let _ = self.authed(|access| self.api.stats(access, &id, &report));
        }
        Ok(self.current_stats())
    }

    /// Returns the current live statistics for the UI.
    pub fn current_stats(&self) -> ConnectionStats {
        let inner = self.inner.lock().unwrap();
        ConnectionStats {
            download_bps: inner.download_bps,
            upload_bps: inner.upload_bps,
            latency_ms: inner.latency_ms,
            duration_seconds: inner.connected_at.map(|t| t.elapsed().as_secs()).unwrap_or(0),
        }
    }

    /// Keep-alive: renews the lease (and reports stats) while connected.
    pub fn keep_alive(&self) -> Result<()> {
        let connection_id = {
            let inner = self.inner.lock().unwrap();
            if !inner.state.is_connected() {
                return Ok(());
            }
            match &inner.active_connection_id {
                Some(id) => id.clone(),
                None => return Ok(()),
            }
        };
        let report = self
            .provider
            .stats()
            .map(|s| StatsReport { rx_bps: s.rx_bytes as i64, tx_bps: s.tx_bytes as i64, latency_ms: 0 })
            .unwrap_or_default();
        self.authed(|access| self.api.stats(access, &connection_id, &report))
    }

    // --- internals ---------------------------------------------------------

    /// Runs `f` with a valid access token, refreshing once on a 401.
    fn authed<T>(&self, f: impl Fn(&str) -> Result<T>) -> Result<T> {
        let access = self.access_token()?;
        match f(&access) {
            Err(CoreError::Api { status: 401, .. }) => {
                let refreshed = self.do_refresh()?;
                f(&refreshed)
            }
            other => other,
        }
    }

    fn access_token(&self) -> Result<String> {
        {
            let inner = self.inner.lock().unwrap();
            if !inner.access_token.is_empty() {
                return Ok(inner.access_token.clone());
            }
        }
        self.do_refresh()
    }

    fn do_refresh(&self) -> Result<String> {
        let refresh = {
            let inner = self.inner.lock().unwrap();
            match &inner.session {
                Some(s) if !s.refresh_token.is_empty() => s.refresh_token.clone(),
                _ => return Err(CoreError::NotAuthenticated),
            }
        };
        let tokens = self.api.refresh(&refresh)?;
        let mut inner = self.inner.lock().unwrap();
        inner.access_token = tokens.access_token.clone();
        Ok(tokens.access_token)
    }

    fn set_state(&self, state: ConnectionState) {
        self.inner.lock().unwrap().state = state;
    }

    fn fail(&self, err: &CoreError) {
        self.inner.lock().unwrap().state = ConnectionState::Failed(err.to_string());
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::transport::{HttpRequest, HttpResponse};
    use crate::tunnel::{TunnelConfig, TunnelStats};
    use std::sync::{Arc, Mutex};

    /// Transport that returns a canned response per path substring.
    struct ScriptedTransport {
        routes: Vec<(&'static str, u16, String)>,
    }
    impl Transport for ScriptedTransport {
        fn send(&self, req: HttpRequest) -> Result<HttpResponse> {
            for (needle, status, body) in &self.routes {
                if req.url.contains(needle) {
                    return Ok(HttpResponse { status: *status, body: body.clone() });
                }
            }
            Ok(HttpResponse { status: 404, body: r#"{"error":"no route"}"#.into() })
        }
    }

    #[derive(Default)]
    struct FakeProvider {
        calls: Arc<Mutex<Vec<String>>>,
    }
    impl TunnelProvider for FakeProvider {
        fn up(&self, cfg: &TunnelConfig) -> Result<()> {
            self.calls.lock().unwrap().push(format!("up:{}", cfg.endpoint));
            Ok(())
        }
        fn down(&self) -> Result<()> {
            self.calls.lock().unwrap().push("down".into());
            Ok(())
        }
        fn stats(&self) -> Result<TunnelStats> {
            Ok(TunnelStats { rx_bytes: 10, tx_bytes: 20, last_handshake_epoch: 1 })
        }
    }

    fn scripted() -> ScriptedTransport {
        ScriptedTransport {
            routes: vec![
                ("/v1/enroll", 201, r#"{"access_token":"AT","refresh_token":"RT","user_id":"u1","device_id":"d1"}"#.into()),
                ("/v1/connections", 201, r#"{"connection_id":"lease-1","server":{"name":"de-fra-1","country":"Germany","city":"Frankfurt","endpoint":"203.0.113.9:51820","public_key":"GWPUB="},"assigned_ip":"10.7.1.5/32","dns":["9.9.9.9"],"allowed_ips":["0.0.0.0/0","::/0"],"persistent_keepalive":25,"expires_at":"2026-01-01T00:00:00Z"}"#.into()),
            ],
        }
    }

    #[test]
    fn enroll_then_connect_then_disconnect() {
        let calls = Arc::new(Mutex::new(Vec::new()));
        let provider = FakeProvider { calls: calls.clone() };
        let client = VpnClient::new("http://cp", Box::new(scripted()), Box::new(provider));

        assert_eq!(client.state(), ConnectionState::Disconnected);

        let session = client.enroll("inv", "a@b.co", None, "laptop", "macos").unwrap();
        assert_eq!(session.device_id, "d1");
        assert!(!session.private_key.is_empty(), "keypair generated");
        // The public key derives from the private key that was generated.
        assert_eq!(keys::public_from_private(&session.private_key).unwrap(), session.public_key);

        let summary = client.connect("de").unwrap();
        assert_eq!(summary.server_name, "de-fra-1");
        assert_eq!(summary.assigned_ip, "10.7.1.5/32");
        assert_eq!(client.state(), ConnectionState::Connected);
        assert_eq!(client.current_server().unwrap().public_key, "GWPUB=");

        client.disconnect().unwrap();
        assert_eq!(client.state(), ConnectionState::Disconnected);

        let log = calls.lock().unwrap().clone();
        assert_eq!(log, vec!["up:203.0.113.9:51820".to_string(), "down".to_string()]);
    }

    #[test]
    fn prepare_connection_returns_config_and_defers_connected() {
        let client = VpnClient::new("http://cp", Box::new(scripted()), Box::new(FakeProvider::default()));
        client.enroll("inv", "a@b.co", None, "l", "macos").unwrap();

        let (summary, config) = client.prepare_connection("de").unwrap();
        assert_eq!(summary.assigned_ip, "10.7.1.5/32");
        // The native layer receives a full WireGuard config.
        assert_eq!(config.peer_public_key, "GWPUB=");
        assert_eq!(config.endpoint, "203.0.113.9:51820");
        assert_eq!(config.addresses, vec!["10.7.1.5/32"]);
        assert!(!config.private_key.is_empty());
        // State is Connecting until the platform confirms the tunnel is up.
        assert_eq!(client.state(), ConnectionState::Connecting);

        client.mark_connected();
        assert_eq!(client.state(), ConnectionState::Connected);

        // And disconnect still releases the lease recorded during prepare.
        client.disconnect().unwrap();
        assert_eq!(client.state(), ConnectionState::Disconnected);
    }

    #[test]
    fn measure_latency_times_tcp_handshake() {
        use std::net::TcpListener;
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        let addr = listener.local_addr().unwrap().to_string();
        std::thread::spawn(move || {
            for _ in listener.incoming() {
                break;
            }
        });

        let client = VpnClient::new("http://cp", Box::new(scripted()), Box::new(FakeProvider::default()));
        client.inner.lock().unwrap().probe_addr = Some(addr);
        let latency = client.measure_latency();
        assert!(latency.is_some(), "should measure a real latency via TCP handshake");
        assert!(latency.unwrap() < 1000, "localhost latency should be small");
    }

    #[test]
    fn measure_latency_none_without_probe() {
        let client = VpnClient::new("http://cp", Box::new(scripted()), Box::new(FakeProvider::default()));
        assert_eq!(client.measure_latency(), None);
    }

    #[test]
    fn rate_computes_and_guards() {
        assert_eq!(rate(0, 1000, 1.0), 1000);
        assert_eq!(rate(1000, 3000, 2.0), 1000);
        assert_eq!(rate(5000, 0, 1.0), 0); // counter reset guarded
        assert_eq!(rate(0, 1000, 0.0), 0); // zero dt guarded
    }

    #[test]
    fn report_stats_is_noop_when_disconnected() {
        let client = VpnClient::new("http://cp", Box::new(scripted()), Box::new(FakeProvider::default()));
        let s = client.report_tunnel_stats(100, 200, Some(20)).unwrap();
        assert_eq!(s, ConnectionStats::default());
    }

    #[test]
    fn stats_track_duration_after_connect() {
        let client = VpnClient::new("http://cp", Box::new(scripted()), Box::new(FakeProvider::default()));
        client.enroll("inv", "a@b.co", None, "l", "macos").unwrap();
        client.connect("de").unwrap();
        // Fresh connection: rates zero, duration defined (>=0), latency unknown.
        let s = client.current_stats();
        assert_eq!(s.download_bps, 0);
        assert_eq!(s.latency_ms, 0);
    }

    #[test]
    fn connect_requires_enrollment() {
        let client = VpnClient::new("http://cp", Box::new(scripted()), Box::new(FakeProvider::default()));
        match client.connect("de") {
            Err(CoreError::NotAuthenticated) => {}
            other => panic!("want NotAuthenticated, got {other:?}"),
        }
        assert_eq!(client.state(), ConnectionState::Disconnected);
    }

    #[test]
    fn connect_api_failure_sets_failed_state() {
        // enroll works, but /v1/connections returns 503.
        let transport = ScriptedTransport {
            routes: vec![
                ("/v1/enroll", 201, r#"{"access_token":"AT","refresh_token":"RT","device_id":"d1"}"#.into()),
                ("/v1/connections", 503, r#"{"error":"no healthy gateway"}"#.into()),
            ],
        };
        let client = VpnClient::new("http://cp", Box::new(transport), Box::new(FakeProvider::default()));
        client.enroll("inv", "a@b.co", None, "l", "ios").unwrap();
        match client.connect("de") {
            Err(CoreError::Api { status: 503, .. }) => {}
            other => panic!("want 503 Api error, got {other:?}"),
        }
        assert!(matches!(client.state(), ConnectionState::Failed(_)));
    }
}

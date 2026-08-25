//! The `VpnClient` façade: the single object each platform's UI drives. It owns
//! the session, the connection state machine, and the orchestration of
//! enroll → connect → disconnect, delegating network I/O to a `Transport` and
//! the OS tunnel to a `TunnelProvider`.

use std::sync::Mutex;

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
        }
        Ok((summary, config))
    }

    /// Marks the tunnel fully established (called by the platform once its native
    /// VPN provider reports the tunnel is up).
    pub fn mark_connected(&self) {
        let mut inner = self.inner.lock().unwrap();
        if inner.state.can_transition_to(&ConnectionState::Connected) {
            inner.state = ConnectionState::Connected;
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
        Ok(())
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

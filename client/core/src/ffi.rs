//! UniFFI bindings for the platform apps (Swift on Apple, Kotlin on Android).
//!
//! This module is compiled only with the `ffi` feature and isolates all UniFFI
//! surface here so the rest of the core stays binding-agnostic. It exposes a
//! single `AevoraClient` object plus plain record/enum mirrors of the core
//! types. The client uses the built-in `ureq` HTTP transport, so the platform
//! provides no networking — only the native tunnel, which it establishes from
//! the `FfiTunnelConfig` returned by `prepare_connection`.

use std::sync::Arc;

use crate::client::ConnectionSummary;
use crate::error::CoreError;
use crate::state::ConnectionState;
use crate::transport::UreqTransport;
use crate::tunnel::{TunnelConfig, TunnelProvider, TunnelStats};
use crate::{Session, VpnClient};

/// A tunnel provider that does nothing: the FFI flow uses `prepare_connection`
/// + `mark_connected`, so the OS tunnel is driven natively, not through here.
struct NoopProvider;
impl TunnelProvider for NoopProvider {
    fn up(&self, _config: &TunnelConfig) -> crate::error::Result<()> {
        Ok(())
    }
    fn down(&self) -> crate::error::Result<()> {
        Ok(())
    }
    fn stats(&self) -> crate::error::Result<TunnelStats> {
        Ok(TunnelStats::default())
    }
}

/// Errors surfaced across the FFI boundary.
#[derive(Debug, thiserror::Error, uniffi::Error)]
pub enum FfiError {
    #[error("api error {status}: {message}")]
    Api { status: u16, message: String },
    #[error("transport error: {message}")]
    Transport { message: String },
    #[error("not authenticated")]
    NotAuthenticated,
    #[error("invalid state")]
    InvalidState,
    #[error("tunnel error: {message}")]
    Tunnel { message: String },
    #[error("{message}")]
    Other { message: String },
}

impl From<CoreError> for FfiError {
    fn from(e: CoreError) -> Self {
        match e {
            CoreError::Api { status, message } => FfiError::Api { status, message },
            CoreError::Transport(m) => FfiError::Transport { message: m },
            CoreError::NotAuthenticated => FfiError::NotAuthenticated,
            CoreError::InvalidTransition { .. } => FfiError::InvalidState,
            CoreError::Tunnel(m) => FfiError::Tunnel { message: m },
            CoreError::Decode(m) => FfiError::Other { message: m },
        }
    }
}

/// Persisted session (the platform stores the private key in its keystore and
/// the refresh token in secure storage).
#[derive(uniffi::Record)]
pub struct FfiSession {
    pub device_id: String,
    pub user_id: String,
    pub refresh_token: String,
    pub private_key: String,
    pub public_key: String,
}

impl From<Session> for FfiSession {
    fn from(s: Session) -> Self {
        FfiSession {
            device_id: s.device_id,
            user_id: s.user_id,
            refresh_token: s.refresh_token,
            private_key: s.private_key,
            public_key: s.public_key,
        }
    }
}
impl From<FfiSession> for Session {
    fn from(s: FfiSession) -> Self {
        Session {
            device_id: s.device_id,
            user_id: s.user_id,
            refresh_token: s.refresh_token,
            private_key: s.private_key,
            public_key: s.public_key,
        }
    }
}

#[derive(uniffi::Record)]
pub struct FfiLocation {
    pub code: String,
    pub country: String,
    pub available: bool,
    pub servers: i64,
}

/// The WireGuard tunnel configuration the native layer establishes.
#[derive(uniffi::Record)]
pub struct FfiTunnelConfig {
    pub private_key: String,
    pub addresses: Vec<String>,
    pub dns: Vec<String>,
    pub peer_public_key: String,
    pub endpoint: String,
    pub allowed_ips: Vec<String>,
    pub persistent_keepalive: u16,
}

impl From<TunnelConfig> for FfiTunnelConfig {
    fn from(c: TunnelConfig) -> Self {
        FfiTunnelConfig {
            private_key: c.private_key,
            addresses: c.addresses,
            dns: c.dns,
            peer_public_key: c.peer_public_key,
            endpoint: c.endpoint,
            allowed_ips: c.allowed_ips,
            persistent_keepalive: c.persistent_keepalive,
        }
    }
}

/// The result of `prepare_connection`: a UI summary plus the tunnel config.
#[derive(uniffi::Record)]
pub struct FfiConnection {
    pub connection_id: String,
    pub server_name: String,
    pub country: String,
    pub city: String,
    pub endpoint: String,
    pub assigned_ip: String,
    pub expires_at: String,
    pub config: FfiTunnelConfig,
}

/// Live connection statistics (real measurements from the OS tunnel).
#[derive(uniffi::Record)]
pub struct FfiStats {
    pub download_bps: u64,
    pub upload_bps: u64,
    pub latency_ms: u32,
    pub duration_seconds: u64,
}

impl From<crate::ConnectionStats> for FfiStats {
    fn from(s: crate::ConnectionStats) -> Self {
        FfiStats {
            download_bps: s.download_bps,
            upload_bps: s.upload_bps,
            latency_ms: s.latency_ms,
            duration_seconds: s.duration_seconds,
        }
    }
}

/// The connection state for the UI.
#[derive(uniffi::Enum)]
pub enum FfiState {
    Disconnected,
    Connecting,
    Connected,
    Disconnecting,
    Failed { reason: String },
}

impl From<ConnectionState> for FfiState {
    fn from(s: ConnectionState) -> Self {
        match s {
            ConnectionState::Disconnected => FfiState::Disconnected,
            ConnectionState::Connecting => FfiState::Connecting,
            ConnectionState::Connected => FfiState::Connected,
            ConnectionState::Disconnecting => FfiState::Disconnecting,
            ConnectionState::Failed(reason) => FfiState::Failed { reason },
        }
    }
}

/// The single object the platform UI drives.
#[derive(uniffi::Object)]
pub struct AevoraClient {
    inner: VpnClient,
}

#[uniffi::export]
impl AevoraClient {
    /// Creates a client pointed at the control-plane base URL.
    #[uniffi::constructor]
    pub fn new(base_url: String) -> Arc<Self> {
        let inner = VpnClient::new(base_url, Box::new(UreqTransport), Box::new(NoopProvider));
        Arc::new(Self { inner })
    }

    /// Restores a persisted session on launch.
    pub fn restore(&self, session: FfiSession) {
        self.inner.restore(session.into());
    }

    /// Enrolls with an invite; the returned session must be persisted.
    pub fn enroll(
        &self,
        invite_code: String,
        email: String,
        display_name: Option<String>,
        device_name: String,
        platform: String,
    ) -> Result<FfiSession, FfiError> {
        Ok(self
            .inner
            .enroll(&invite_code, &email, display_name, &device_name, &platform)?
            .into())
    }

    /// Lists selectable countries.
    pub fn locations(&self) -> Result<Vec<FfiLocation>, FfiError> {
        Ok(self
            .inner
            .locations()?
            .into_iter()
            .map(|l| FfiLocation { code: l.code, country: l.country, available: l.available, servers: l.servers })
            .collect())
    }

    /// Selects a gateway, leases an address, and returns the tunnel config for
    /// the native layer to establish. State becomes Connecting.
    pub fn prepare_connection(&self, country_code: String) -> Result<FfiConnection, FfiError> {
        let (summary, config): (ConnectionSummary, TunnelConfig) = self.inner.prepare_connection(&country_code)?;
        Ok(FfiConnection {
            connection_id: summary.connection_id,
            server_name: summary.server_name,
            country: summary.country,
            city: summary.city,
            endpoint: summary.endpoint,
            assigned_ip: summary.assigned_ip,
            expires_at: summary.expires_at,
            config: config.into(),
        })
    }

    /// Called by the platform once the native tunnel is up.
    pub fn mark_connected(&self) {
        self.inner.mark_connected();
    }

    /// Called by the platform if the native tunnel failed or dropped.
    pub fn mark_failed(&self, reason: String) {
        self.inner.mark_failed(&reason);
    }

    /// Releases the lease (disconnect).
    pub fn disconnect(&self) -> Result<(), FfiError> {
        Ok(self.inner.disconnect()?)
    }

    /// Renews the lease / reports stats while connected.
    pub fn keep_alive(&self) -> Result<(), FfiError> {
        Ok(self.inner.keep_alive()?)
    }

    /// Feeds the OS tunnel's cumulative byte counters (and optional measured
    /// latency), computes real rates, renews the lease, and returns live stats.
    pub fn report_tunnel_stats(&self, rx_bytes: u64, tx_bytes: u64, latency_ms: Option<u32>) -> Result<FfiStats, FfiError> {
        Ok(self.inner.report_tunnel_stats(rx_bytes, tx_bytes, latency_ms)?.into())
    }

    /// Returns the current live statistics for the UI.
    pub fn current_stats(&self) -> FfiStats {
        self.inner.current_stats().into()
    }

    /// The current connection state.
    pub fn state(&self) -> FfiState {
        self.inner.state().into()
    }
}

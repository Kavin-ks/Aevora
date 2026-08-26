//! Wire types matching the Aevora control-plane API (see docs/design).

use serde::{Deserialize, Serialize};

/// Device metadata sent at enrollment / device registration.
#[derive(Debug, Clone, Serialize)]
pub struct DeviceRegistration {
    pub name: String,
    pub platform: String,
    pub public_key: String,
}

/// Body of POST /v1/enroll.
#[derive(Debug, Clone, Serialize)]
pub struct EnrollRequest {
    pub invite_code: String,
    pub email: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub display_name: Option<String>,
    pub device: DeviceRegistration,
}

/// Token payload returned by enroll / refresh.
#[derive(Debug, Clone, Deserialize)]
pub struct TokenResponse {
    #[serde(default)]
    pub access_token: String,
    #[serde(default)]
    pub token_type: String,
    #[serde(default)]
    pub expires_in: i64,
    #[serde(default)]
    pub refresh_token: String,
    #[serde(default)]
    pub user_id: String,
    #[serde(default)]
    pub device_id: String,
}

/// A country the user can pick.
#[derive(Debug, Clone, Deserialize, PartialEq)]
pub struct Location {
    pub code: String,
    pub country: String,
    pub available: bool,
    #[serde(default)]
    pub servers: i64,
}

#[derive(Debug, Clone, Deserialize)]
pub(crate) struct LocationsResponse {
    pub locations: Vec<Location>,
}

/// Body of POST /v1/connections.
#[derive(Debug, Clone, Serialize)]
pub struct ConnectRequest {
    pub country_code: String,
    pub device_id: String,
}

/// The gateway a connection was placed on.
#[derive(Debug, Clone, Deserialize, PartialEq)]
pub struct Server {
    pub name: String,
    pub country: String,
    #[serde(default)]
    pub city: String,
    pub endpoint: String,
    pub public_key: String,
}

/// Response from POST /v1/connections — everything needed to bring up the tunnel.
#[derive(Debug, Clone, Deserialize)]
pub struct ConnectionResponse {
    pub connection_id: String,
    pub server: Server,
    pub assigned_ip: String,
    #[serde(default)]
    pub assigned_ip6: Option<String>,
    pub dns: Vec<String>,
    pub allowed_ips: Vec<String>,
    #[serde(default = "default_keepalive")]
    pub persistent_keepalive: u16,
    #[serde(default)]
    pub expires_at: String,
    /// Gateway in-tunnel address:port for the latency probe (may be empty).
    #[serde(default)]
    pub probe_addr: Option<String>,
}

fn default_keepalive() -> u16 {
    25
}

/// Body of POST /v1/connections/{id}/stats (also the lease keep-alive).
#[derive(Debug, Clone, Serialize, Default)]
pub struct StatsReport {
    pub rx_bps: i64,
    pub tx_bps: i64,
    pub latency_ms: i64,
}

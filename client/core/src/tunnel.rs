//! Tunnel-config assembly and the native tunnel-provider interface.
//!
//! The core produces a `TunnelConfig` (platform-agnostic). Each platform
//! implements `TunnelProvider` over its native VPN framework (NetworkExtension
//! on iOS/macOS, VpnService on Android, WireGuard-NT on Windows) to bring the
//! tunnel up from that config.

use crate::error::Result;
use crate::keys::KeyPair;
use crate::model::ConnectionResponse;

/// Everything the native tunnel layer needs to configure WireGuard. Mirrors a
/// WireGuard `[Interface]` + `[Peer]` pair.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct TunnelConfig {
    /// Client private key (base64). Stays on device.
    pub private_key: String,
    /// Interface addresses, e.g. ["10.7.1.5/32", "fd07::5/128"].
    pub addresses: Vec<String>,
    /// DNS resolvers to use inside the tunnel.
    pub dns: Vec<String>,
    /// Gateway (peer) public key.
    pub peer_public_key: String,
    /// Gateway endpoint host:port.
    pub endpoint: String,
    /// Allowed IPs (full tunnel: 0.0.0.0/0, ::/0).
    pub allowed_ips: Vec<String>,
    /// Keep-alive seconds.
    pub persistent_keepalive: u16,
}

/// Live counters read back from the native tunnel.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct TunnelStats {
    pub rx_bytes: u64,
    pub tx_bytes: u64,
    /// Unix epoch seconds of the last WireGuard handshake (0 if none yet).
    pub last_handshake_epoch: u64,
}

/// Implemented natively per platform. The core calls these; it never touches the
/// OS network stack itself.
pub trait TunnelProvider: Send + Sync {
    fn up(&self, config: &TunnelConfig) -> Result<()>;
    fn down(&self) -> Result<()>;
    fn stats(&self) -> Result<TunnelStats>;
}

/// Builds the tunnel config from the device keypair and the control plane's
/// connection response.
pub fn build_config(keypair: &KeyPair, conn: &ConnectionResponse) -> TunnelConfig {
    let mut addresses = vec![conn.assigned_ip.clone()];
    if let Some(v6) = &conn.assigned_ip6 {
        if !v6.is_empty() {
            addresses.push(v6.clone());
        }
    }
    TunnelConfig {
        private_key: keypair.private_key.clone(),
        addresses,
        dns: conn.dns.clone(),
        peer_public_key: conn.server.public_key.clone(),
        endpoint: conn.server.endpoint.clone(),
        allowed_ips: conn.allowed_ips.clone(),
        persistent_keepalive: conn.persistent_keepalive,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::model::Server;

    fn sample_conn() -> ConnectionResponse {
        ConnectionResponse {
            connection_id: "lease-1".into(),
            server: Server {
                name: "de-fra-1".into(),
                country: "Germany".into(),
                city: "Frankfurt".into(),
                endpoint: "203.0.113.9:51820".into(),
                public_key: "GWPUB=".into(),
            },
            assigned_ip: "10.7.1.5/32".into(),
            assigned_ip6: Some("fd07::5/128".into()),
            dns: vec!["9.9.9.9".into()],
            allowed_ips: vec!["0.0.0.0/0".into(), "::/0".into()],
            persistent_keepalive: 25,
            expires_at: "".into(),
            probe_addr: None,
        }
    }

    #[test]
    fn assembles_full_tunnel_config() {
        let kp = KeyPair { private_key: "PRIV=".into(), public_key: "PUB=".into() };
        let cfg = build_config(&kp, &sample_conn());
        assert_eq!(cfg.private_key, "PRIV=");
        assert_eq!(cfg.addresses, vec!["10.7.1.5/32", "fd07::5/128"]);
        assert_eq!(cfg.peer_public_key, "GWPUB=");
        assert_eq!(cfg.endpoint, "203.0.113.9:51820");
        assert_eq!(cfg.allowed_ips, vec!["0.0.0.0/0", "::/0"]);
        assert_eq!(cfg.persistent_keepalive, 25);
    }

    #[test]
    fn omits_v6_when_absent() {
        let mut conn = sample_conn();
        conn.assigned_ip6 = None;
        let kp = KeyPair { private_key: "p".into(), public_key: "P".into() };
        let cfg = build_config(&kp, &conn);
        assert_eq!(cfg.addresses, vec!["10.7.1.5/32"]);
    }
}

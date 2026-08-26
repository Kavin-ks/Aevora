//! Live smoke test: drive the real client core (ureq transport) against a
//! running control plane. Requires the `net` feature.
//!
//!   CONTROL_URL=http://127.0.0.1:8099 INVITE=<code> \
//!     cargo run --example smoke --features net
//!
//! Uses a no-op tunnel provider (this exercises the control-plane path, not the
//! OS tunnel).

use aevora_core::{Result, TunnelConfig, TunnelProvider, TunnelStats, UreqTransport, VpnClient};

struct NoopProvider;
impl TunnelProvider for NoopProvider {
    fn up(&self, _c: &TunnelConfig) -> Result<()> {
        Ok(())
    }
    fn down(&self) -> Result<()> {
        Ok(())
    }
    fn stats(&self) -> Result<TunnelStats> {
        Ok(TunnelStats::default())
    }
}

fn main() {
    let base = std::env::var("CONTROL_URL").expect("set CONTROL_URL");
    let invite = std::env::var("INVITE").expect("set INVITE");
    let country = std::env::var("COUNTRY").unwrap_or_else(|_| "de".into());

    let client = VpnClient::new(base, Box::new(UreqTransport), Box::new(NoopProvider));

    let session = client
        .enroll(&invite, "smoke@example.com", None, "smoke-cli", "cli")
        .expect("enroll");
    println!("enrolled: device={} pubkey={}", session.device_id, session.public_key);

    let locations = client.locations().expect("locations");
    println!("locations: {locations:?}");

    let summary = client.connect(&country).expect("connect");
    println!(
        "CONNECTED: server={} ({}) endpoint={} ip={} state={}",
        summary.server_name,
        summary.country,
        summary.endpoint,
        summary.assigned_ip,
        client.state()
    );

    client.keep_alive().expect("keep_alive");
    println!("keep-alive ok");

    // Feed tunnel counters; the core computes rates and reports to /stats.
    let stats = client
        .report_tunnel_stats(1_000_000, 500_000, Some(18))
        .expect("report stats");
    println!(
        "stats reported: latency={}ms duration={}s (rates 0 on first sample)",
        stats.latency_ms, stats.duration_seconds
    );

    client.disconnect().expect("disconnect");
    println!("disconnected, state={}", client.state());
}

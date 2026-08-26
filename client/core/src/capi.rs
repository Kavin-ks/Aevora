//! C ABI for platforms without a UniFFI binding generator (Windows/.NET via
//! P/Invoke). Compiled with the `capi` feature. Complex values are returned as
//! JSON C strings (freed with `aevora_string_free`); errors are returned as
//! `{"error": "..."}`. This is a thin wrapper over the same `VpnClient` — no
//! business logic lives here.

use std::ffi::{c_char, c_void, CStr, CString};

use serde_json::json;

use crate::transport::UreqTransport;
use crate::tunnel::{TunnelConfig, TunnelProvider, TunnelStats};
use crate::VpnClient;

struct NoopProvider;
impl TunnelProvider for NoopProvider {
    fn up(&self, _c: &TunnelConfig) -> crate::error::Result<()> {
        Ok(())
    }
    fn down(&self) -> crate::error::Result<()> {
        Ok(())
    }
    fn stats(&self) -> crate::error::Result<TunnelStats> {
        Ok(TunnelStats::default())
    }
}

fn cstr(p: *const c_char) -> String {
    if p.is_null() {
        return String::new();
    }
    unsafe { CStr::from_ptr(p).to_string_lossy().into_owned() }
}

fn ret(s: String) -> *mut c_char {
    CString::new(s).unwrap_or_default().into_raw()
}

fn err(msg: impl std::fmt::Display) -> *mut c_char {
    ret(json!({ "error": msg.to_string() }).to_string())
}

fn client<'a>(h: *mut c_void) -> Option<&'a VpnClient> {
    if h.is_null() {
        None
    } else {
        Some(unsafe { &*(h as *const VpnClient) })
    }
}

/// Creates a client for the control-plane base URL. Returns an opaque handle.
#[no_mangle]
pub extern "C" fn aevora_client_new(base_url: *const c_char) -> *mut c_void {
    let client = VpnClient::new(cstr(base_url), Box::new(UreqTransport), Box::new(NoopProvider));
    Box::into_raw(Box::new(client)) as *mut c_void
}

/// Frees a client handle.
#[no_mangle]
pub extern "C" fn aevora_client_free(h: *mut c_void) {
    if !h.is_null() {
        unsafe { drop(Box::from_raw(h as *mut VpnClient)) };
    }
}

/// Frees a string returned by this API.
#[no_mangle]
pub extern "C" fn aevora_string_free(s: *mut c_char) {
    if !s.is_null() {
        unsafe { drop(CString::from_raw(s)) };
    }
}

/// Enrolls; returns the session JSON (persist it) or an error object.
#[no_mangle]
pub extern "C" fn aevora_enroll(
    h: *mut c_void,
    invite: *const c_char,
    email: *const c_char,
    device_name: *const c_char,
) -> *mut c_char {
    let Some(c) = client(h) else { return err("null handle") };
    match c.enroll(&cstr(invite), &cstr(email), None, &cstr(device_name), "windows") {
        Ok(s) => ret(json!({
            "device_id": s.device_id, "user_id": s.user_id,
            "refresh_token": s.refresh_token,
            "private_key": s.private_key, "public_key": s.public_key,
        })
        .to_string()),
        Err(e) => err(e),
    }
}

/// Restores a persisted session (JSON as returned by enroll).
#[no_mangle]
pub extern "C" fn aevora_restore(h: *mut c_void, session_json: *const c_char) -> *mut c_char {
    let Some(c) = client(h) else { return err("null handle") };
    let v: serde_json::Value = match serde_json::from_str(&cstr(session_json)) {
        Ok(v) => v,
        Err(e) => return err(e),
    };
    let get = |k: &str| v.get(k).and_then(|x| x.as_str()).unwrap_or("").to_string();
    c.restore(crate::Session {
        device_id: get("device_id"),
        user_id: get("user_id"),
        refresh_token: get("refresh_token"),
        private_key: get("private_key"),
        public_key: get("public_key"),
    });
    ret(json!({"ok": true}).to_string())
}

/// Lists locations as a JSON array.
#[no_mangle]
pub extern "C" fn aevora_locations(h: *mut c_void) -> *mut c_char {
    let Some(c) = client(h) else { return err("null handle") };
    match c.locations() {
        Ok(locs) => {
            let arr: Vec<_> = locs
                .into_iter()
                .map(|l| json!({"code": l.code, "country": l.country, "available": l.available, "servers": l.servers}))
                .collect();
            ret(json!({ "locations": arr }).to_string())
        }
        Err(e) => err(e),
    }
}

/// Prepares a connection: returns the summary + WireGuard tunnel config JSON for
/// the native layer to establish. State becomes Connecting.
#[no_mangle]
pub extern "C" fn aevora_prepare_connection(h: *mut c_void, country: *const c_char) -> *mut c_char {
    let Some(c) = client(h) else { return err("null handle") };
    match c.prepare_connection(&cstr(country)) {
        Ok((summary, config)) => ret(json!({
            "connection_id": summary.connection_id,
            "server_name": summary.server_name,
            "country": summary.country,
            "city": summary.city,
            "endpoint": summary.endpoint,
            "assigned_ip": summary.assigned_ip,
            "expires_at": summary.expires_at,
            "config": {
                "private_key": config.private_key,
                "addresses": config.addresses,
                "dns": config.dns,
                "peer_public_key": config.peer_public_key,
                "endpoint": config.endpoint,
                "allowed_ips": config.allowed_ips,
                "persistent_keepalive": config.persistent_keepalive,
            }
        })
        .to_string()),
        Err(e) => err(e),
    }
}

/// Marks the native tunnel established.
#[no_mangle]
pub extern "C" fn aevora_mark_connected(h: *mut c_void) {
    if let Some(c) = client(h) {
        c.mark_connected();
    }
}

/// Releases the lease (disconnect).
#[no_mangle]
pub extern "C" fn aevora_disconnect(h: *mut c_void) -> *mut c_char {
    let Some(c) = client(h) else { return err("null handle") };
    match c.disconnect() {
        Ok(_) => ret(json!({"ok": true}).to_string()),
        Err(e) => err(e),
    }
}

/// Reports tunnel byte counters; returns live stats JSON (latency measured).
#[no_mangle]
pub extern "C" fn aevora_report_stats(h: *mut c_void, rx_bytes: u64, tx_bytes: u64) -> *mut c_char {
    let Some(c) = client(h) else { return err("null handle") };
    match c.report_tunnel_stats(rx_bytes, tx_bytes, None) {
        Ok(s) => ret(stats_json(&s)),
        Err(e) => err(e),
    }
}

/// Returns the current live stats JSON.
#[no_mangle]
pub extern "C" fn aevora_current_stats(h: *mut c_void) -> *mut c_char {
    let Some(c) = client(h) else { return err("null handle") };
    ret(stats_json(&c.current_stats()))
}

/// Returns the connection state as JSON, e.g. {"state":"connected"}.
#[no_mangle]
pub extern "C" fn aevora_state(h: *mut c_void) -> *mut c_char {
    let Some(c) = client(h) else { return err("null handle") };
    ret(json!({ "state": c.state().to_string() }).to_string())
}

fn stats_json(s: &crate::ConnectionStats) -> String {
    json!({
        "download_bps": s.download_bps,
        "upload_bps": s.upload_bps,
        "latency_ms": s.latency_ms,
        "duration_seconds": s.duration_seconds,
    })
    .to_string()
}

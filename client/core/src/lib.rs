//! # aevora-core
//!
//! The shared client core for the Aevora VPN. Every platform app (macOS,
//! Windows, Android, iOS) links this library and drives a [`VpnClient`]; the
//! platform supplies only two things natively: an HTTP [`Transport`] and a
//! [`TunnelProvider`] over its OS VPN framework. All the security-critical
//! logic — API calls, token handling, WireGuard key generation, the connection
//! state machine, and tunnel-config assembly — lives here, written once.

pub mod api;
pub mod client;
pub mod error;
pub mod keys;
pub mod model;
pub mod state;
pub mod transport;
pub mod tunnel;

pub use client::{ConnectionStats, ConnectionSummary, Session, VpnClient};
pub use error::{CoreError, Result};
pub use keys::KeyPair;
pub use model::{Location, Server};
pub use state::ConnectionState;
pub use transport::{HttpRequest, HttpResponse, Transport};
pub use tunnel::{TunnelConfig, TunnelProvider, TunnelStats};

#[cfg(feature = "net")]
pub use transport::UreqTransport;

#[cfg(feature = "ffi")]
mod ffi;

#[cfg(feature = "ffi")]
uniffi::setup_scaffolding!();

#[cfg(feature = "capi")]
mod capi;

use thiserror::Error;

/// Errors surfaced by the client core.
#[derive(Debug, Error)]
pub enum CoreError {
    /// The transport (network layer) failed before an HTTP response was formed.
    #[error("transport error: {0}")]
    Transport(String),

    /// The control plane returned a non-2xx response.
    #[error("api error {status}: {message}")]
    Api { status: u16, message: String },

    /// A response body could not be decoded.
    #[error("decode error: {0}")]
    Decode(String),

    /// The client is not authenticated (no session / refresh failed).
    #[error("not authenticated")]
    NotAuthenticated,

    /// An operation was attempted from an invalid connection state.
    #[error("invalid state transition: {from} -> {to}")]
    InvalidTransition { from: String, to: String },

    /// The tunnel provider (native VPN layer) failed.
    #[error("tunnel error: {0}")]
    Tunnel(String),
}

pub type Result<T> = std::result::Result<T, CoreError>;

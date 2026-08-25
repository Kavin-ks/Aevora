//! The connection state machine shared by every platform's UI.

use std::fmt;

/// The lifecycle of a VPN connection as the UI sees it.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum ConnectionState {
    Disconnected,
    Connecting,
    Connected,
    Disconnecting,
    Failed(String),
}

impl fmt::Display for ConnectionState {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            ConnectionState::Disconnected => write!(f, "disconnected"),
            ConnectionState::Connecting => write!(f, "connecting"),
            ConnectionState::Connected => write!(f, "connected"),
            ConnectionState::Disconnecting => write!(f, "disconnecting"),
            ConnectionState::Failed(msg) => write!(f, "failed({msg})"),
        }
    }
}

impl ConnectionState {
    /// Whether a transition from `self` to `next` is allowed.
    pub fn can_transition_to(&self, next: &ConnectionState) -> bool {
        use ConnectionState::*;
        matches!(
            (self, next),
            (Disconnected, Connecting)
                | (Connecting, Connected)
                | (Connecting, Failed(_))
                | (Connecting, Disconnecting)
                | (Connected, Disconnecting)
                | (Connected, Failed(_))
                | (Disconnecting, Disconnected)
                | (Disconnecting, Failed(_))
                | (Failed(_), Connecting)
                | (Failed(_), Disconnected)
        )
    }

    /// True once the tunnel is fully up.
    pub fn is_connected(&self) -> bool {
        matches!(self, ConnectionState::Connected)
    }

    /// True while a connect/disconnect is in flight.
    pub fn is_transitional(&self) -> bool {
        matches!(self, ConnectionState::Connecting | ConnectionState::Disconnecting)
    }
}

#[cfg(test)]
mod tests {
    use super::ConnectionState::*;

    #[test]
    fn valid_transitions() {
        assert!(Disconnected.can_transition_to(&Connecting));
        assert!(Connecting.can_transition_to(&Connected));
        assert!(Connecting.can_transition_to(&Failed("x".into())));
        assert!(Connected.can_transition_to(&Disconnecting));
        assert!(Disconnecting.can_transition_to(&Disconnected));
        assert!(Failed("x".into()).can_transition_to(&Connecting));
    }

    #[test]
    fn invalid_transitions() {
        assert!(!Disconnected.can_transition_to(&Connected)); // must go via Connecting
        assert!(!Connected.can_transition_to(&Connecting));
        assert!(!Disconnected.can_transition_to(&Disconnecting));
        assert!(!Connected.can_transition_to(&Connected));
    }

    #[test]
    fn helpers() {
        assert!(Connected.is_connected());
        assert!(Connecting.is_transitional());
        assert!(Disconnecting.is_transitional());
        assert!(!Disconnected.is_transitional());
    }
}

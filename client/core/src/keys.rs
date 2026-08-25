//! WireGuard key generation. Keys are X25519 (Curve25519), base64-encoded — the
//! same representation `wg` uses. The private key is generated on-device and is
//! meant to be stored in the platform keystore; only the public key is sent to
//! the control plane.

use base64::engine::general_purpose::STANDARD;
use base64::Engine as _;
use x25519_dalek::{PublicKey, StaticSecret};

/// A WireGuard keypair, base64-encoded.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct KeyPair {
    pub private_key: String,
    pub public_key: String,
}

/// Generates a fresh keypair from the OS CSPRNG.
pub fn generate() -> KeyPair {
    let secret = StaticSecret::random_from_rng(rand::rngs::OsRng);
    keypair_from_secret(secret)
}

/// Derives the matching public key for an existing base64 private key. Used when
/// the platform restores a persisted private key.
pub fn public_from_private(private_b64: &str) -> Option<String> {
    let bytes = STANDARD.decode(private_b64).ok()?;
    let arr: [u8; 32] = bytes.try_into().ok()?;
    let secret = StaticSecret::from(arr);
    Some(STANDARD.encode(PublicKey::from(&secret).as_bytes()))
}

fn keypair_from_secret(secret: StaticSecret) -> KeyPair {
    let public = PublicKey::from(&secret);
    KeyPair {
        private_key: STANDARD.encode(secret.to_bytes()),
        public_key: STANDARD.encode(public.as_bytes()),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn generates_distinct_valid_keys() {
        let a = generate();
        let b = generate();
        assert_ne!(a.private_key, b.private_key, "keys must be unique");

        // Both keys must be 32 bytes when decoded.
        assert_eq!(STANDARD.decode(&a.private_key).unwrap().len(), 32);
        assert_eq!(STANDARD.decode(&a.public_key).unwrap().len(), 32);
    }

    #[test]
    fn public_key_derives_from_private() {
        let kp = generate();
        let derived = public_from_private(&kp.private_key).unwrap();
        assert_eq!(derived, kp.public_key, "public must derive from private");
    }

    #[test]
    fn public_from_bad_private_is_none() {
        assert!(public_from_private("not-base64!!!").is_none());
        assert!(public_from_private("dG9vc2hvcnQ=").is_none()); // decodes but != 32 bytes
    }
}

# Security model

Aevora treats security as a first-class requirement. This document is the
reference for the trust boundaries, credentials, and the security checklist.

## Trust boundaries

Four independently authenticated principals:

| Principal | Authenticates with | Can do |
|-----------|--------------------|--------|
| **User** | email + password → short-lived JWT; or invite → JWT | Manage own devices/connections |
| **Device** | per-device WireGuard keypair + refresh token | Establish tunnels for its user |
| **Gateway** | enrollment secret (once) → per-node token (hashed) | Register, heartbeat, fetch its peers |
| **Admin** | admin bearer token | Mint invites, view fleet |

Authorization is enforced server-side: users can only touch their own devices
and connections; a node token only exposes that gateway's peer set; admin/
enrollment secrets are never sent to clients.

## Cryptography & keys

- **VPN:** WireGuard only (Curve25519 · ChaCha20-Poly1305 · BLAKE2s). No custom
  protocol.
- **Device keys:** the WireGuard private key is generated **on the device** (in
  `aevora-core`) and stored in the platform keystore (Apple Keychain, Android
  Keystore, Windows DPAPI). **Only the public key is ever transmitted.**
- **Passwords:** hashed with **argon2id** (memory-hard), PHC-encoded, verified in
  constant time.
- **Tokens:** access tokens are short-lived **HS256 JWTs** (algorithm pinned on
  verify). Refresh tokens and gateway node tokens are stored only as **SHA-256
  hashes** — a DB leak exposes no usable token.
- **Pre-shared keys:** the gateway kit supports an optional per-peer PSK; the
  control-plane-managed flow omits it by default to avoid storing a symmetric
  secret at rest (WireGuard is secure without it).

## Transport

- All client ↔ control-plane traffic is **TLS** (Caddy / your load balancer).
- Behind a proxy, `AEVORA_TRUST_PROXY=true` uses `X-Forwarded-For`; enable it
  **only** behind a proxy you control (the header is otherwise spoofable).

## Abuse resistance

- **Rate limiting** per client IP on `login` / `enroll` / `refresh` (429 on
  abuse).
- **Device revocation** immediately revokes the device's refresh tokens and
  expires its active connections (peer removed on the gateway).
- **No orphaned peers:** peers on a gateway are exactly its active leases;
  reconnects dedupe, disconnects/expiry/failover all reap.

## Logging & privacy

- Structured logs record operational events only. They **never** log passwords,
  private keys, tokens, or WireGuard private keys.
- No browsing/DNS/traffic logs. Session records are minimal and time-bounded.

## Never commit

Private keys, passwords, API keys, signing certificates, provisioning profiles,
VPS credentials, production DB credentials, WireGuard configs containing private
keys, or `.env` files with secrets. The repo `.gitignore` excludes these; a
secrets audit runs before every commit.

## Security checklist

- [ ] TLS enforced for all control-plane traffic.
- [ ] JWT / enrollment / admin secrets strong, random, out of git.
- [ ] Device private keys only in platform keystores; public key transmitted.
- [ ] argon2id for passwords; JWT alg pinned; tokens stored hashed.
- [ ] Rate limiting on auth endpoints; `TrustProxy` only behind a real proxy.
- [ ] Device revocation tested (tokens revoked + peer removed).
- [ ] `/metrics` and DB not publicly reachable.
- [ ] OS VPN entitlements minimal; hardened runtime / sandbox on.
- [ ] Node token files `chmod 600`; enrollment secret only on gateways.
- [ ] Dependencies patched; migrations reviewed.

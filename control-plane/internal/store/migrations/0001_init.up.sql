-- Aevora control-plane schema — initial.
-- Only PUBLIC keys are ever stored here; client private keys never leave devices.

CREATE EXTENSION IF NOT EXISTS pgcrypto;   -- gen_random_uuid()
CREATE EXTENSION IF NOT EXISTS citext;     -- case-insensitive email

CREATE TABLE users (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email        citext UNIQUE NOT NULL,
    display_name text,
    status       text NOT NULL DEFAULT 'active' CHECK (status IN ('active','suspended')),
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE invite_codes (
    code       text PRIMARY KEY,
    note       text,
    created_by uuid REFERENCES users(id),
    used_by    uuid REFERENCES users(id),
    expires_at timestamptz,
    used_at    timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE devices (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         text NOT NULL,
    platform     text NOT NULL CHECK (platform IN ('ios','macos','android','windows','linux','cli')),
    public_key   text NOT NULL UNIQUE,          -- WireGuard client public key
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz,
    revoked_at   timestamptz
);

CREATE TABLE locations (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code         text NOT NULL UNIQUE,          -- 'de', 'sg'
    country_name text NOT NULL,                 -- 'Germany'
    enabled      boolean NOT NULL DEFAULT true,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE gateways (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    location_id       uuid NOT NULL REFERENCES locations(id) ON DELETE RESTRICT,
    name              text NOT NULL UNIQUE,     -- 'de-fra-1'
    city              text,
    public_key        text NOT NULL,            -- gateway WireGuard public key
    endpoint_host     text NOT NULL,            -- ip or hostname
    endpoint_port     integer NOT NULL DEFAULT 51820,
    wg_subnet_v4      cidr NOT NULL,
    wg_subnet_v6      cidr,
    capacity          integer NOT NULL DEFAULT 250 CHECK (capacity > 0),
    status            text NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('pending','healthy','unhealthy','draining','disabled')),
    -- live metrics, updated by the agent heartbeat
    active_peers      integer NOT NULL DEFAULT 0,
    cpu_pct           real    NOT NULL DEFAULT 0,
    rx_bps            bigint  NOT NULL DEFAULT 0,
    tx_bps            bigint  NOT NULL DEFAULT 0,
    agent_token_hash  text,                     -- sha256 of the node bearer token
    last_heartbeat_at timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_gateways_location ON gateways(location_id);
CREATE INDEX idx_gateways_status   ON gateways(status);

CREATE TABLE leases (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    gateway_id        uuid NOT NULL REFERENCES gateways(id) ON DELETE CASCADE,
    device_id         uuid NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    client_public_key text NOT NULL,            -- snapshot of the device key at connect time
    assigned_ip_v4    inet NOT NULL,
    assigned_ip_v6    inet,
    preshared_key     text,
    state             text NOT NULL DEFAULT 'active'
                        CHECK (state IN ('active','released','expired')),
    created_at        timestamptz NOT NULL DEFAULT now(),
    expires_at        timestamptz NOT NULL,
    released_at       timestamptz
);
-- At most one active lease per IP on a gateway: makes double-allocation impossible.
CREATE UNIQUE INDEX uq_lease_gw_ip_active ON leases(gateway_id, assigned_ip_v4)
    WHERE state = 'active';
CREATE INDEX idx_leases_device ON leases(device_id);
CREATE INDEX idx_leases_state  ON leases(state);

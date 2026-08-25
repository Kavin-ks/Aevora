-- Phase 1b — identity: refresh tokens for session continuity.
-- users, devices and invite_codes already exist from 0001. Only the hash of a
-- refresh token is stored, so a database leak does not expose usable tokens.

CREATE TABLE refresh_tokens (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id  uuid NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    token_hash text NOT NULL UNIQUE,          -- sha256 of the refresh token
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz
);
CREATE INDEX idx_refresh_device ON refresh_tokens(device_id);

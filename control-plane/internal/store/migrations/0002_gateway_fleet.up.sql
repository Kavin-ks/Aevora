-- Phase 1c — gateway fleet: richer metadata for selection, and a token index.

ALTER TABLE gateways
    ADD COLUMN region         text,
    ADD COLUMN latitude       double precision,
    ADD COLUMN longitude      double precision,
    ADD COLUMN bandwidth_mbps integer;

-- Store cpu as double precision so it scans cleanly as float64.
ALTER TABLE gateways ALTER COLUMN cpu_pct TYPE double precision;

-- Heartbeat and deregister look a gateway up by the SHA-256 of its node token.
CREATE INDEX idx_gateways_token ON gateways(agent_token_hash);

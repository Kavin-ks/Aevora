-- Idempotent development seed. Applied only when AEVORA_DEV_SEED=1.
-- Gives GET /v1/locations something real to return before any agent registers.
-- The gateway keys/endpoints are placeholders for local UI/API work only.

INSERT INTO locations (code, country_name) VALUES
    ('de', 'Germany'),
    ('sg', 'Singapore'),
    ('us', 'United States')
ON CONFLICT (code) DO NOTHING;

INSERT INTO gateways (location_id, name, city, public_key, endpoint_host, endpoint_port, wg_subnet_v4, wg_subnet_v6, capacity, status, active_peers, last_heartbeat_at)
SELECT l.id, v.name, v.city, v.public_key, v.host, 51820, v.subnet4::cidr, v.subnet6::cidr, v.capacity, 'healthy', v.active, now()
FROM (VALUES
    ('de', 'de-fra-1', 'Frankfurt', 'DEV_PLACEHOLDER_PUBKEY_FRA1=', '192.0.2.11', '10.7.1.0/24', 'fd07:0007:1::/64', 250, 40),
    ('de', 'de-fra-2', 'Frankfurt', 'DEV_PLACEHOLDER_PUBKEY_FRA2=', '192.0.2.12', '10.7.2.0/24', 'fd07:0007:2::/64', 250, 210),
    ('sg', 'sg-sin-1', 'Singapore', 'DEV_PLACEHOLDER_PUBKEY_SIN1=', '192.0.2.21', '10.7.3.0/24', 'fd07:0007:3::/64', 250, 12)
) AS v(loc, name, city, public_key, host, subnet4, subnet6, capacity, active)
JOIN locations l ON l.code = v.loc
ON CONFLICT (name) DO NOTHING;

-- A US location with no healthy gateway, to exercise "unavailable" in the UI.

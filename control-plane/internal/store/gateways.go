package store

import (
	"context"
	"errors"
	"time"

	"github.com/aevora/control-plane/internal/auth"
	"github.com/aevora/control-plane/internal/model"
	"github.com/jackc/pgx/v5"
)

// ErrInvalidToken is returned when a node token does not match any gateway.
var ErrInvalidToken = errors.New("invalid node token")

// nilIfEmpty converts an empty string to nil so pgx writes SQL NULL.
func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// nilIfZero converts a zero int to nil so pgx writes SQL NULL.
func nilIfZero(n int) any {
	if n == 0 {
		return nil
	}
	return n
}

// RegisterGateway upserts a gateway by its unique name and (implicitly) its
// country. It issues a fresh node token, stores only the token's hash, and
// returns the gateway plus the one-time plaintext token. Re-registration (agent
// restart) updates metadata, resets status to pending, and rotates the token.
func (s *Store) RegisterGateway(ctx context.Context, r model.GatewayRegistration) (model.Gateway, string, error) {
	tokenPlain, err := auth.GenerateToken()
	if err != nil {
		return model.Gateway{}, "", err
	}
	tokenHash := auth.HashToken(tokenPlain)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return model.Gateway{}, "", err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after Commit

	var locationID string
	if err := tx.QueryRow(ctx,
		`INSERT INTO locations (code, country_name)
		 VALUES ($1, $2)
		 ON CONFLICT (code) DO UPDATE SET country_name = EXCLUDED.country_name
		 RETURNING id::text`,
		r.CountryCode, r.CountryName).Scan(&locationID); err != nil {
		return model.Gateway{}, "", err
	}

	var g model.Gateway
	g.LocationID = locationID
	g.LocationCode = r.CountryCode
	g.CountryName = r.CountryName
	g.Name = r.Name
	g.City = r.City
	g.Region = r.Region
	g.PublicKey = r.PublicKey
	g.EndpointHost = r.EndpointHost
	g.EndpointPort = r.EndpointPort
	g.Capacity = r.Capacity
	g.BandwidthMbps = r.BandwidthMbps
	g.Latitude = r.Latitude
	g.Longitude = r.Longitude

	err = tx.QueryRow(ctx,
		`INSERT INTO gateways
		   (location_id, name, city, region, public_key, endpoint_host, endpoint_port,
		    wg_subnet_v4, wg_subnet_v6, capacity, bandwidth_mbps, latitude, longitude,
		    status, agent_token_hash)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,'pending',$14)
		 ON CONFLICT (name) DO UPDATE SET
		    location_id      = EXCLUDED.location_id,
		    city             = EXCLUDED.city,
		    region           = EXCLUDED.region,
		    public_key       = EXCLUDED.public_key,
		    endpoint_host    = EXCLUDED.endpoint_host,
		    endpoint_port    = EXCLUDED.endpoint_port,
		    wg_subnet_v4     = EXCLUDED.wg_subnet_v4,
		    wg_subnet_v6     = EXCLUDED.wg_subnet_v6,
		    capacity         = EXCLUDED.capacity,
		    bandwidth_mbps   = EXCLUDED.bandwidth_mbps,
		    latitude         = EXCLUDED.latitude,
		    longitude        = EXCLUDED.longitude,
		    status           = 'pending',
		    agent_token_hash = EXCLUDED.agent_token_hash
		 RETURNING id::text, status`,
		locationID, r.Name, nilIfEmpty(r.City), nilIfEmpty(r.Region), r.PublicKey,
		r.EndpointHost, r.EndpointPort, r.WGSubnetV4, nilIfEmpty(r.WGSubnetV6),
		r.Capacity, nilIfZero(r.BandwidthMbps), r.Latitude, r.Longitude, tokenHash,
	).Scan(&g.ID, &g.Status)
	if err != nil {
		return model.Gateway{}, "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return model.Gateway{}, "", err
	}
	return g, tokenPlain, nil
}

// Heartbeat records live metrics for the gateway identified by tokenHash and
// marks it healthy. Returns ErrInvalidToken if no gateway matches.
func (s *Store) Heartbeat(ctx context.Context, tokenHash string, m model.GatewayMetrics) (model.Gateway, error) {
	var g model.Gateway
	err := s.pool.QueryRow(ctx,
		`UPDATE gateways
		 SET active_peers = $2, cpu_pct = $3, rx_bps = $4, tx_bps = $5,
		     last_heartbeat_at = now(), status = 'healthy'
		 WHERE agent_token_hash = $1 AND status <> 'disabled'
		 RETURNING id::text, name, status`,
		tokenHash, m.ActivePeers, m.CPUPct, m.RxBps, m.TxBps).
		Scan(&g.ID, &g.Name, &g.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Gateway{}, ErrInvalidToken
	}
	if err != nil {
		return model.Gateway{}, err
	}
	return g, nil
}

// DeregisterGateway marks the gateway identified by tokenHash disabled (a clean
// agent shutdown). Returns ErrInvalidToken if no gateway matches.
func (s *Store) DeregisterGateway(ctx context.Context, tokenHash string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE gateways SET status = 'disabled' WHERE agent_token_hash = $1`, tokenHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrInvalidToken
	}
	return nil
}

// MarkStaleUnhealthy flips healthy gateways whose last heartbeat is older than
// ttl to unhealthy, so they leave the selection pool. Returns how many changed.
func (s *Store) MarkStaleUnhealthy(ctx context.Context, ttl time.Duration) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE gateways SET status = 'unhealthy'
		 WHERE status = 'healthy'
		   AND (last_heartbeat_at IS NULL
		        OR last_heartbeat_at < now() - make_interval(secs => $1))`,
		ttl.Seconds())
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

const gatewaySelectCols = `
	g.id::text, l.id::text, l.code, l.country_name, g.name,
	COALESCE(g.city,''), COALESCE(g.region,''), g.public_key,
	g.endpoint_host, g.endpoint_port, g.capacity, COALESCE(g.bandwidth_mbps,0),
	g.latitude, g.longitude, g.status, g.active_peers, g.cpu_pct,
	g.rx_bps, g.tx_bps, g.last_heartbeat_at`

func scanGateway(rows pgx.Rows) (model.Gateway, error) {
	var g model.Gateway
	err := rows.Scan(
		&g.ID, &g.LocationID, &g.LocationCode, &g.CountryName, &g.Name,
		&g.City, &g.Region, &g.PublicKey, &g.EndpointHost, &g.EndpointPort,
		&g.Capacity, &g.BandwidthMbps, &g.Latitude, &g.Longitude, &g.Status,
		&g.ActivePeers, &g.CPUPct, &g.RxBps, &g.TxBps, &g.LastHeartbeatAt)
	return g, err
}

// ListGateways returns every gateway with its metadata and live metrics,
// ordered by country then name. For the control-plane admin view.
func (s *Store) ListGateways(ctx context.Context) ([]model.Gateway, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+gatewaySelectCols+`
		 FROM gateways g JOIN locations l ON l.id = g.location_id
		 ORDER BY l.country_name, g.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Gateway
	for rows.Next() {
		g, err := scanGateway(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// SelectableGateways returns the healthy, fresh-heartbeat gateways with spare
// capacity in a location — the candidate set the selection policy ranks. This
// is the foundation the connect flow (Phase 1d) builds on.
func (s *Store) SelectableGateways(ctx context.Context, countryCode string, ttl time.Duration) ([]model.Gateway, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+gatewaySelectCols+`
		 FROM gateways g JOIN locations l ON l.id = g.location_id
		 WHERE l.code = $1
		   AND l.enabled = true
		   AND g.status = 'healthy'
		   AND g.last_heartbeat_at >= now() - make_interval(secs => $2)
		   AND g.active_peers < g.capacity`,
		countryCode, ttl.Seconds())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Gateway
	for rows.Next() {
		g, err := scanGateway(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

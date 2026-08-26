package store

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"strconv"
	"time"

	"github.com/aevora/control-plane/internal/model"
	"github.com/aevora/control-plane/internal/selection"
	"github.com/jackc/pgx/v5"
)

// Connection errors.
var (
	ErrDeviceNotFound     = errors.New("device not found or revoked")
	ErrNoGatewayAvailable = errors.New("no healthy gateway available in location")
	ErrGatewayFull        = errors.New("gateway address pool exhausted")
)

// gwCandidate is a selectable gateway plus the address info needed to allocate.
type gwCandidate struct {
	g        model.Gateway
	subnetV4 string
	subnetV6 *string
}

// CreateConnection selects the least-loaded healthy gateway in the country,
// leases a free address for the device, and records the lease. The peer is
// programmed on the gateway asynchronously by the node agent, which reconciles
// against GatewayPeers. Load is measured by active lease count (the control
// plane's authoritative view), and allocation is serialized per gateway with a
// row lock so two concurrent connects cannot claim the same address.
func (s *Store) CreateConnection(ctx context.Context, userID, deviceID, country string, leaseTTL, heartbeatTTL time.Duration) (model.Connection, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return model.Connection{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// 1. The device must belong to the caller and not be revoked.
	var clientPub string
	err = tx.QueryRow(ctx,
		`SELECT public_key FROM devices WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL`,
		deviceID, userID).Scan(&clientPub)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Connection{}, ErrDeviceNotFound
	}
	if err != nil {
		return model.Connection{}, err
	}

	// 1b. Release any existing active lease for this device (reconnect / dedupe),
	//     so a device never holds two peers and no orphan is left behind.
	if _, err := tx.Exec(ctx,
		`UPDATE leases SET state = 'released', released_at = now()
		 WHERE device_id = $1 AND state = 'active'`, deviceID); err != nil {
		return model.Connection{}, err
	}

	// 2. Candidate gateways in the country, healthy and heartbeating, with load
	//    measured as active lease count, plus advertised bandwidth for scoring.
	rows, err := tx.Query(ctx,
		`SELECT g.id::text, g.name, COALESCE(g.city,''), l.country_name, g.public_key,
		        g.endpoint_host, g.endpoint_port, g.wg_subnet_v4::text, g.wg_subnet_v6::text,
		        g.capacity, COALESCE(g.bandwidth_mbps,0), g.rx_bps, g.tx_bps,
		        (SELECT count(*) FROM leases le WHERE le.gateway_id = g.id AND le.state = 'active') AS active
		 FROM gateways g JOIN locations l ON l.id = g.location_id
		 WHERE l.code = $1 AND l.enabled = true AND g.status = 'healthy'
		   AND g.last_heartbeat_at >= now() - make_interval(secs => $2)`,
		country, heartbeatTTL.Seconds())
	if err != nil {
		return model.Connection{}, err
	}
	byID := map[string]gwCandidate{}
	var forSelect []model.Gateway
	for rows.Next() {
		var c gwCandidate
		var port int
		if err := rows.Scan(&c.g.ID, &c.g.Name, &c.g.City, &c.g.CountryName, &c.g.PublicKey,
			&c.g.EndpointHost, &port, &c.subnetV4, &c.subnetV6, &c.g.Capacity,
			&c.g.BandwidthMbps, &c.g.RxBps, &c.g.TxBps, &c.g.ActivePeers); err != nil {
			rows.Close()
			return model.Connection{}, err
		}
		c.g.EndpointPort = port
		c.g.Status = model.GatewayHealthy
		byID[c.g.ID] = c
		forSelect = append(forSelect, c.g)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return model.Connection{}, err
	}

	best, ok := selection.Best(forSelect)
	if !ok {
		return model.Connection{}, ErrNoGatewayAvailable
	}
	chosen := byID[best.ID]

	// 3. Lock the chosen gateway so allocation is race-free.
	if _, err := tx.Exec(ctx, `SELECT 1 FROM gateways WHERE id = $1 FOR UPDATE`, chosen.g.ID); err != nil {
		return model.Connection{}, err
	}

	// 4. Gather addresses already in use on this gateway.
	used, err := usedHostAddrs(ctx, tx, chosen.g.ID)
	if err != nil {
		return model.Connection{}, err
	}

	// 5. Allocate the lowest free address.
	v4, v6, err := allocateHost(chosen.subnetV4, chosen.subnetV6, used)
	if err != nil {
		return model.Connection{}, err
	}

	// 6. Record the lease (snapshotting the client public key).
	var conn model.Connection
	err = tx.QueryRow(ctx,
		`INSERT INTO leases (gateway_id, device_id, client_public_key, assigned_ip_v4, assigned_ip_v6, expires_at)
		 VALUES ($1, $2, $3, $4, $5, now() + make_interval(secs => $6))
		 RETURNING id::text, expires_at`,
		chosen.g.ID, deviceID, clientPub, v4, v6, leaseTTL.Seconds()).
		Scan(&conn.ID, &conn.ExpiresAt)
	if err != nil {
		return model.Connection{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Connection{}, err
	}

	conn.GatewayName = chosen.g.Name
	conn.Country = chosen.g.CountryName
	conn.City = chosen.g.City
	conn.Endpoint = net.JoinHostPort(chosen.g.EndpointHost, strconv.Itoa(chosen.g.EndpointPort))
	conn.GatewayPublicKey = chosen.g.PublicKey
	conn.AssignedIPv4 = v4 + "/32"
	if v6 != nil {
		s6 := *v6 + "/128"
		conn.AssignedIPv6 = &s6
	}
	return conn, nil
}

// usedHostAddrs returns the set of active-lease IPv4 addresses on a gateway,
// keyed by their 32-bit integer value.
func usedHostAddrs(ctx context.Context, tx pgx.Tx, gatewayID string) (map[uint32]bool, error) {
	// host() strips the netmask pgx stores on inet values, so ParseIP works.
	rows, err := tx.Query(ctx,
		`SELECT host(assigned_ip_v4) FROM leases WHERE gateway_id = $1 AND state = 'active'`, gatewayID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	used := map[uint32]bool{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		if ip := net.ParseIP(s).To4(); ip != nil {
			used[binary.BigEndian.Uint32(ip)] = true
		}
	}
	return used, rows.Err()
}

// allocateHost returns the lowest free host address in the v4 subnet (skipping
// the network address and the gateway's .1), plus the matching v6 host address
// if a v6 subnet is configured. Host-only strings are returned (no prefix).
func allocateHost(cidrV4 string, cidrV6 *string, used map[uint32]bool) (string, *string, error) {
	_, netV4, err := net.ParseCIDR(cidrV4)
	if err != nil {
		return "", nil, err
	}
	base := binary.BigEndian.Uint32(netV4.IP.To4())
	ones, bits := netV4.Mask.Size()
	size := uint32(1) << uint(bits-ones)

	var netV6 *net.IPNet
	if cidrV6 != nil && *cidrV6 != "" {
		if _, n6, err := net.ParseCIDR(*cidrV6); err == nil {
			netV6 = n6
		}
	}

	// h in [2, size-2]: skip .0 (network), .1 (gateway), and the broadcast.
	for h := uint32(2); h < size-1; h++ {
		cand := base + h
		if used[cand] {
			continue
		}
		v4b := make(net.IP, net.IPv4len)
		binary.BigEndian.PutUint32(v4b, cand)
		v4 := v4b.String()

		var v6 *string
		if netV6 != nil {
			c6 := make(net.IP, net.IPv6len)
			copy(c6, netV6.IP.To16())
			binary.BigEndian.PutUint32(c6[12:16], binary.BigEndian.Uint32(c6[12:16])+h)
			s := c6.String()
			v6 = &s
		}
		return v4, v6, nil
	}
	return "", nil, ErrGatewayFull
}

// GatewayPeers returns the desired peer set for the gateway identified by
// tokenHash: the active, unexpired leases. The node agent reconciles against it.
func (s *Store) GatewayPeers(ctx context.Context, tokenHash string) ([]model.Peer, error) {
	var gatewayID string
	err := s.pool.QueryRow(ctx,
		`SELECT id::text FROM gateways WHERE agent_token_hash = $1 AND status <> 'disabled'`,
		tokenHash).Scan(&gatewayID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidToken
	}
	if err != nil {
		return nil, err
	}

	// host() yields the bare address (no netmask) so we append /32 and /128 once.
	rows, err := s.pool.Query(ctx,
		`SELECT client_public_key, host(assigned_ip_v4), host(assigned_ip_v6)
		 FROM leases WHERE gateway_id = $1 AND state = 'active' AND expires_at > now()`,
		gatewayID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var peers []model.Peer
	for rows.Next() {
		var pub, v4 string
		var v6 *string
		if err := rows.Scan(&pub, &v4, &v6); err != nil {
			return nil, err
		}
		allowed := []string{v4 + "/32"}
		if v6 != nil && *v6 != "" {
			allowed = append(allowed, *v6+"/128")
		}
		peers = append(peers, model.Peer{PublicKey: pub, AllowedIPs: allowed})
	}
	return peers, rows.Err()
}

// ReleaseConnection releases an active lease the caller owns. ErrNotFound if it
// is not active or not the caller's.
func (s *Store) ReleaseConnection(ctx context.Context, userID, leaseID string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE leases SET state = 'released', released_at = now()
		 WHERE id = $1 AND state = 'active'
		   AND device_id IN (SELECT id FROM devices WHERE user_id = $2)`,
		leaseID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RenewConnection extends an active lease the caller owns (the keep-alive that
// stops the reaper from expiring a live connection). Returns the new expiry.
func (s *Store) RenewConnection(ctx context.Context, userID, leaseID string, leaseTTL time.Duration) (time.Time, error) {
	var expiresAt time.Time
	err := s.pool.QueryRow(ctx,
		`UPDATE leases SET expires_at = now() + make_interval(secs => $3)
		 WHERE id = $1 AND state = 'active'
		   AND device_id IN (SELECT id FROM devices WHERE user_id = $2)
		 RETURNING expires_at`,
		leaseID, userID, leaseTTL.Seconds()).Scan(&expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, ErrNotFound
	}
	return expiresAt, err
}

// ExpireStaleLeases flips active leases past their expiry to expired, freeing
// the address and causing the agent to remove the peer on its next reconcile.
func (s *Store) ExpireStaleLeases(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE leases SET state = 'expired' WHERE state = 'active' AND expires_at < now()`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ExpireLeasesOnUnhealthyGateways expires active leases whose gateway is no
// longer healthy (unhealthy/draining/disabled). This is the failover trigger:
// the client's tunnel to a dead gateway is broken anyway, so the lease is
// released and the client reconnects — landing on a healthy gateway.
func (s *Store) ExpireLeasesOnUnhealthyGateways(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE leases SET state = 'expired'
		 WHERE state = 'active'
		   AND gateway_id IN (SELECT id FROM gateways WHERE status <> 'healthy')`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

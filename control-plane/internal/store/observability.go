package store

import "context"

// FleetSnapshot returns the gateway count by status and the number of active
// leases, for periodic metrics export.
func (s *Store) FleetSnapshot(ctx context.Context) (gatewaysByStatus map[string]int, activeSessions int, err error) {
	gatewaysByStatus = map[string]int{}
	rows, err := s.pool.Query(ctx, `SELECT status, count(*) FROM gateways GROUP BY status`)
	if err != nil {
		return nil, 0, err
	}
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			rows.Close()
			return nil, 0, err
		}
		gatewaysByStatus[status] = n
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM leases WHERE state = 'active'`).Scan(&activeSessions); err != nil {
		return nil, 0, err
	}
	return gatewaysByStatus, activeSessions, nil
}

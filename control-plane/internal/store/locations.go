package store

import (
	"context"

	"github.com/aevora/control-plane/internal/model"
)

// ListLocations returns every enabled location with its total and healthy
// gateway counts, ordered by country name. Availability is derived by callers
// from HealthyCount.
func (s *Store) ListLocations(ctx context.Context) ([]model.Location, error) {
	const q = `
		SELECT l.id::text, l.code, l.country_name, l.enabled,
		       COUNT(g.id)                                        AS total,
		       COUNT(g.id) FILTER (WHERE g.status = 'healthy')    AS healthy
		FROM locations l
		LEFT JOIN gateways g ON g.location_id = l.id
		WHERE l.enabled = true
		GROUP BY l.id, l.code, l.country_name, l.enabled
		ORDER BY l.country_name`

	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Location
	for rows.Next() {
		var l model.Location
		if err := rows.Scan(&l.ID, &l.Code, &l.CountryName, &l.Enabled, &l.ServerCount, &l.HealthyCount); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

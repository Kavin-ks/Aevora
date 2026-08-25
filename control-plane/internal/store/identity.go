package store

import (
	"context"
	"errors"
	"time"

	"github.com/aevora/control-plane/internal/auth"
	"github.com/aevora/control-plane/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Identity errors.
var (
	ErrInviteInvalid  = errors.New("invite is invalid, expired, or already used")
	ErrDeviceKeyTaken = errors.New("device public key already registered")
	ErrNotFound       = errors.New("not found")
)

// isUniqueViolation reports whether err is a Postgres unique-constraint error.
func isUniqueViolation(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}

// newRefreshToken inserts a refresh token for a device (storing only its hash)
// and returns the one-time plaintext. Runs inside the caller's transaction.
func newRefreshToken(ctx context.Context, tx pgx.Tx, deviceID string, ttl time.Duration) (string, error) {
	plain, err := auth.GenerateToken()
	if err != nil {
		return "", err
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO refresh_tokens (device_id, token_hash, expires_at)
		 VALUES ($1, $2, now() + make_interval(secs => $3))`,
		deviceID, auth.HashToken(plain), ttl.Seconds())
	if err != nil {
		return "", err
	}
	return plain, nil
}

// Enroll validates and consumes an invite, upserts the user by email, creates
// the first device, and issues a refresh token. Returns the user, device and
// one-time refresh token. The access JWT is minted by the caller.
func (s *Store) Enroll(ctx context.Context, req model.EnrollRequest, refreshTTL time.Duration) (model.User, model.Device, string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return model.User{}, model.Device{}, "", err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after Commit

	// Validate + lock the invite row.
	var usedAt, expiresAt *time.Time
	err = tx.QueryRow(ctx,
		`SELECT used_at, expires_at FROM invite_codes WHERE code = $1 FOR UPDATE`,
		req.InviteCode).Scan(&usedAt, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.User{}, model.Device{}, "", ErrInviteInvalid
	}
	if err != nil {
		return model.User{}, model.Device{}, "", err
	}
	if usedAt != nil || (expiresAt != nil && expiresAt.Before(time.Now())) {
		return model.User{}, model.Device{}, "", ErrInviteInvalid
	}

	// Upsert the user by email (a returning person can enroll more devices).
	var u model.User
	err = tx.QueryRow(ctx,
		`INSERT INTO users (email, display_name)
		 VALUES ($1, $2)
		 ON CONFLICT (email) DO UPDATE
		   SET display_name = COALESCE(EXCLUDED.display_name, users.display_name)
		 RETURNING id::text, email, COALESCE(display_name,''), status, created_at`,
		req.Email, nilIfEmpty(req.DisplayName)).
		Scan(&u.ID, &u.Email, &u.DisplayName, &u.Status, &u.CreatedAt)
	if err != nil {
		return model.User{}, model.Device{}, "", err
	}

	d, err := insertDevice(ctx, tx, u.ID, req.Device)
	if err != nil {
		return model.User{}, model.Device{}, "", err
	}

	if _, err := tx.Exec(ctx,
		`UPDATE invite_codes SET used_by = $1, used_at = now() WHERE code = $2`,
		u.ID, req.InviteCode); err != nil {
		return model.User{}, model.Device{}, "", err
	}

	refresh, err := newRefreshToken(ctx, tx, d.ID, refreshTTL)
	if err != nil {
		return model.User{}, model.Device{}, "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.User{}, model.Device{}, "", err
	}
	return u, d, refresh, nil
}

// insertDevice inserts a device, mapping a unique-key collision to
// ErrDeviceKeyTaken. Runs inside the caller's transaction.
func insertDevice(ctx context.Context, tx pgx.Tx, userID string, r model.DeviceRegistration) (model.Device, error) {
	d := model.Device{UserID: userID, Name: r.Name, Platform: r.Platform, PublicKey: r.PublicKey}
	err := tx.QueryRow(ctx,
		`INSERT INTO devices (user_id, name, platform, public_key)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id::text, created_at`,
		userID, r.Name, r.Platform, r.PublicKey).Scan(&d.ID, &d.CreatedAt)
	if isUniqueViolation(err) {
		return model.Device{}, ErrDeviceKeyTaken
	}
	if err != nil {
		return model.Device{}, err
	}
	return d, nil
}

// CreateDevice adds another device to an existing user and issues a refresh
// token for it. Returns the device and one-time refresh token.
func (s *Store) CreateDevice(ctx context.Context, userID string, r model.DeviceRegistration, refreshTTL time.Duration) (model.Device, string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return model.Device{}, "", err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	d, err := insertDevice(ctx, tx, userID, r)
	if err != nil {
		return model.Device{}, "", err
	}
	refresh, err := newRefreshToken(ctx, tx, d.ID, refreshTTL)
	if err != nil {
		return model.Device{}, "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Device{}, "", err
	}
	return d, refresh, nil
}

// ListDevices returns a user's devices, newest first.
func (s *Store) ListDevices(ctx context.Context, userID string) ([]model.Device, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, user_id::text, name, platform, public_key,
		        created_at, last_seen_at, revoked_at
		 FROM devices WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Device
	for rows.Next() {
		var d model.Device
		if err := rows.Scan(&d.ID, &d.UserID, &d.Name, &d.Platform, &d.PublicKey,
			&d.CreatedAt, &d.LastSeenAt, &d.RevokedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// RevokeDevice revokes a device the user owns and revokes its refresh tokens.
// Returns ErrNotFound if the device is not the caller's (or already revoked).
func (s *Store) RevokeDevice(ctx context.Context, userID, deviceID string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE devices SET revoked_at = now()
		 WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL`, deviceID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	_, err = s.pool.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = now()
		 WHERE device_id = $1 AND revoked_at IS NULL`, deviceID)
	return err
}

// RefreshAccess validates a refresh token (not expired/revoked, device active)
// and returns the owning user and device. Touches the device's last_seen_at.
func (s *Store) RefreshAccess(ctx context.Context, refreshPlain string) (userID, deviceID string, err error) {
	err = s.pool.QueryRow(ctx,
		`SELECT d.user_id::text, d.id::text
		 FROM refresh_tokens rt
		 JOIN devices d ON d.id = rt.device_id
		 WHERE rt.token_hash = $1
		   AND rt.revoked_at IS NULL
		   AND rt.expires_at > now()
		   AND d.revoked_at IS NULL`,
		auth.HashToken(refreshPlain)).Scan(&userID, &deviceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrInvalidToken
	}
	if err != nil {
		return "", "", err
	}
	_, _ = s.pool.Exec(ctx, `UPDATE devices SET last_seen_at = now() WHERE id = $1`, deviceID)
	return userID, deviceID, nil
}

// CreateInvite mints an invite code (optionally with a note and expiry).
func (s *Store) CreateInvite(ctx context.Context, code, note string, expiresAt *time.Time) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO invite_codes (code, note, expires_at) VALUES ($1, $2, $3)`,
		code, nilIfEmpty(note), expiresAt)
	return err
}

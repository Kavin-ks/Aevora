package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/aevora/control-plane/internal/auth"
	"github.com/aevora/control-plane/internal/model"
	"github.com/aevora/control-plane/internal/store"
)

// tokenResponse is the OAuth-ish token payload returned by enroll/refresh.
type tokenResponse struct {
	AccessToken  string `json:"access_token,omitempty"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	UserID       string `json:"user_id,omitempty"`
	DeviceID     string `json:"device_id,omitempty"`
}

func (s *Server) issueAccess(userID string) (string, error) {
	return auth.IssueJWT(s.cfg.JWTSecret, userID, s.cfg.AccessTTL)
}

// handleEnroll consumes an invite and creates a user + first device, returning
// an access token and a refresh token.
func (s *Server) handleEnroll(w http.ResponseWriter, r *http.Request) {
	if s.cfg.JWTSecret == "" {
		writeError(w, http.StatusServiceUnavailable, "authentication is disabled")
		return
	}
	var req model.EnrollRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if msg, ok := validateEnroll(req); !ok {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	user, device, refresh, err := s.store.Enroll(r.Context(), req, s.cfg.RefreshTTL)
	switch {
	case errors.Is(err, store.ErrInviteInvalid):
		writeError(w, http.StatusForbidden, "invite is invalid, expired, or already used")
		return
	case errors.Is(err, store.ErrDeviceKeyTaken):
		writeError(w, http.StatusConflict, "device public key already registered")
		return
	case err != nil:
		s.log.Error("enroll", "err", err)
		writeError(w, http.StatusInternalServerError, "enrollment failed")
		return
	}

	access, err := s.issueAccess(user.ID)
	if err != nil {
		s.log.Error("issue access token", "err", err)
		writeError(w, http.StatusInternalServerError, "could not issue token")
		return
	}
	writeJSON(w, http.StatusCreated, tokenResponse{
		AccessToken:  access,
		TokenType:    "Bearer",
		ExpiresIn:    int(s.cfg.AccessTTL.Seconds()),
		RefreshToken: refresh,
		UserID:       user.ID,
		DeviceID:     device.ID,
	})
}

func validateEnroll(r model.EnrollRequest) (string, bool) {
	switch {
	case r.InviteCode == "":
		return "invite_code is required", false
	case !strings.Contains(r.Email, "@"):
		return "a valid email is required", false
	case r.Device.Name == "":
		return "device.name is required", false
	case !model.ValidPlatform(r.Device.Platform):
		return "device.platform must be one of ios, macos, android, windows, linux, cli", false
	case r.Device.PublicKey == "":
		return "device.public_key is required", false
	}
	return "", true
}

// refreshRequest is the body of POST /v1/auth/refresh.
type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// handleRefresh exchanges a valid refresh token for a new access token.
func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if s.cfg.JWTSecret == "" {
		writeError(w, http.StatusServiceUnavailable, "authentication is disabled")
		return
	}
	var req refreshRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&req); err != nil || req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "refresh_token is required")
		return
	}

	userID, _, err := s.store.RefreshAccess(r.Context(), req.RefreshToken)
	if errors.Is(err, store.ErrInvalidToken) {
		writeError(w, http.StatusUnauthorized, "invalid or expired refresh token")
		return
	}
	if err != nil {
		s.log.Error("refresh", "err", err)
		writeError(w, http.StatusInternalServerError, "refresh failed")
		return
	}

	access, err := s.issueAccess(userID)
	if err != nil {
		s.log.Error("issue access token", "err", err)
		writeError(w, http.StatusInternalServerError, "could not issue token")
		return
	}
	writeJSON(w, http.StatusOK, tokenResponse{
		AccessToken: access,
		TokenType:   "Bearer",
		ExpiresIn:   int(s.cfg.AccessTTL.Seconds()),
	})
}

// deviceDTO is the wire shape of a device. It exposes the public key (safe) and
// never any private material.
type deviceDTO struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Platform   string  `json:"platform"`
	PublicKey  string  `json:"public_key"`
	CreatedAt  string  `json:"created_at"`
	LastSeenAt *string `json:"last_seen_at,omitempty"`
	Revoked    bool    `json:"revoked"`
}

// handleDeviceList lists the authenticated user's devices.
func (s *Server) handleDeviceList(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.authUser(w, r)
	if !ok {
		return
	}
	devs, err := s.store.ListDevices(r.Context(), userID)
	if err != nil {
		s.log.Error("list devices", "err", err)
		writeError(w, http.StatusInternalServerError, "could not list devices")
		return
	}
	out := make([]deviceDTO, 0, len(devs))
	for _, d := range devs {
		out = append(out, toDeviceDTO(d))
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": out})
}

func toDeviceDTO(d model.Device) deviceDTO {
	dto := deviceDTO{
		ID:        d.ID,
		Name:      d.Name,
		Platform:  d.Platform,
		PublicKey: d.PublicKey,
		CreatedAt: d.CreatedAt.UTC().Format(time.RFC3339),
		Revoked:   d.RevokedAt != nil,
	}
	if d.LastSeenAt != nil {
		s := d.LastSeenAt.UTC().Format(time.RFC3339)
		dto.LastSeenAt = &s
	}
	return dto
}

// handleDeviceCreate registers an additional device for the authenticated user
// and returns a refresh token the new device can use.
func (s *Server) handleDeviceCreate(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.authUser(w, r)
	if !ok {
		return
	}
	var reg model.DeviceRegistration
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&reg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	switch {
	case reg.Name == "":
		writeError(w, http.StatusBadRequest, "name is required")
		return
	case !model.ValidPlatform(reg.Platform):
		writeError(w, http.StatusBadRequest, "platform must be one of ios, macos, android, windows, linux, cli")
		return
	case reg.PublicKey == "":
		writeError(w, http.StatusBadRequest, "public_key is required")
		return
	}

	dev, refresh, err := s.store.CreateDevice(r.Context(), userID, reg, s.cfg.RefreshTTL)
	if errors.Is(err, store.ErrDeviceKeyTaken) {
		writeError(w, http.StatusConflict, "device public key already registered")
		return
	}
	if err != nil {
		s.log.Error("create device", "err", err)
		writeError(w, http.StatusInternalServerError, "could not register device")
		return
	}
	writeJSON(w, http.StatusCreated, tokenResponse{
		TokenType:    "Bearer",
		RefreshToken: refresh,
		DeviceID:     dev.ID,
	})
}

// handleDeviceRevoke revokes one of the authenticated user's devices.
func (s *Server) handleDeviceRevoke(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.authUser(w, r)
	if !ok {
		return
	}
	deviceID := r.PathValue("id")
	err := s.store.RevokeDevice(r.Context(), userID, deviceID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "device not found")
		return
	}
	if err != nil {
		s.log.Error("revoke device", "err", err)
		writeError(w, http.StatusInternalServerError, "could not revoke device")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// inviteCreateRequest is the body of POST /v1/invites.
type inviteCreateRequest struct {
	Note string `json:"note"`
	TTL  string `json:"ttl"` // optional Go duration, e.g. "168h"
}

// handleInviteCreate mints an invite code. Guarded by the admin token; disabled
// (404) when no admin token is configured.
func (s *Server) handleInviteCreate(w http.ResponseWriter, r *http.Request) {
	if s.cfg.AdminToken == "" {
		http.NotFound(w, r)
		return
	}
	if !constEq(bearerToken(r), s.cfg.AdminToken) {
		writeError(w, http.StatusUnauthorized, "invalid admin token")
		return
	}
	var req inviteCreateRequest
	// Body is optional; ignore decode errors on an empty body.
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&req)

	var expiresAt *time.Time
	if req.TTL != "" {
		d, err := time.ParseDuration(req.TTL)
		if err != nil {
			writeError(w, http.StatusBadRequest, "ttl must be a Go duration, e.g. 168h")
			return
		}
		t := time.Now().Add(d)
		expiresAt = &t
	}

	code, err := auth.GenerateToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not generate invite")
		return
	}
	if err := s.store.CreateInvite(r.Context(), code, req.Note, expiresAt); err != nil {
		s.log.Error("create invite", "err", err)
		writeError(w, http.StatusInternalServerError, "could not create invite")
		return
	}
	resp := map[string]any{"code": code}
	if expiresAt != nil {
		resp["expires_at"] = expiresAt.UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusCreated, resp)
}

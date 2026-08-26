package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/aevora/control-plane/internal/auth"
	"github.com/aevora/control-plane/internal/store"
)

// loginRequest is the body of POST /v1/auth/login.
type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// handleLogin authenticates a user by email + password and issues an access
// token. To get a refresh token, the client then registers a device. This
// endpoint is rate-limited (see server wiring) to resist brute force.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if s.cfg.JWTSecret == "" {
		writeError(w, http.StatusServiceUnavailable, "authentication is disabled")
		return
	}
	var req loginRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	userID, hash, status, err := s.store.GetCredentialsByEmail(r.Context(), req.Email)
	// Generic error to avoid user enumeration / distinguishing "no password".
	if errors.Is(err, store.ErrNotFound) || (err == nil && hash == "") {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err != nil {
		s.log.Error("login lookup", "err", err)
		writeError(w, http.StatusInternalServerError, "login failed")
		return
	}
	if status != "active" {
		writeError(w, http.StatusForbidden, "account is not active")
		return
	}
	if auth.VerifyPassword(req.Password, hash) != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
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
		UserID:      userID,
	})
}

// setPasswordRequest is the body of POST /v1/auth/password.
type setPasswordRequest struct {
	Password string `json:"password"`
}

// handleSetPassword sets or changes the authenticated user's password.
func (s *Server) handleSetPassword(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.authUser(w, r)
	if !ok {
		return
	}
	var req setPasswordRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		s.log.Error("hash password", "err", err)
		writeError(w, http.StatusInternalServerError, "could not set password")
		return
	}
	if err := s.store.SetPassword(r.Context(), userID, hash); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		s.log.Error("set password", "err", err)
		writeError(w, http.StatusInternalServerError, "could not set password")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aevora/control-plane/internal/auth"
	"github.com/aevora/control-plane/internal/model"
	"github.com/aevora/control-plane/internal/store"
)

const testJWTSecret = "unit-test-jwt-secret"

// authCfg is a ServerConfig with user auth enabled.
func authCfg() ServerConfig {
	return ServerConfig{JWTSecret: testJWTSecret, AccessTTL: 15 * time.Minute, RefreshTTL: 720 * time.Hour}
}

func bearerReq(method, path, token, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

const validEnrollBody = `{
  "invite_code":"inv-abc","email":"friend@example.com","display_name":"Friend",
  "device":{"name":"laptop","platform":"macos","public_key":"CLIENT_PUBKEY_1="}
}`

func TestEnroll_DisabledWithoutJWTSecret(t *testing.T) {
	rr := httptest.NewRecorder()
	testServer(&fakeStore{}, ServerConfig{}).
		ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/v1/enroll", strings.NewReader(validEnrollBody)))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rr.Code)
	}
}

func TestEnroll_Success(t *testing.T) {
	fs := &fakeStore{
		enrollUser:    model.User{ID: "user-1", Email: "friend@example.com"},
		enrollDevice:  model.Device{ID: "device-1"},
		enrollRefresh: "refresh-token-plain",
	}
	rr := httptest.NewRecorder()
	testServer(fs, authCfg()).
		ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/v1/enroll", strings.NewReader(validEnrollBody)))

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (%s)", rr.Code, rr.Body)
	}
	var body tokenResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.RefreshToken != "refresh-token-plain" {
		t.Errorf("refresh token not returned")
	}
	if body.UserID != "user-1" || body.DeviceID != "device-1" {
		t.Errorf("ids not returned: %+v", body)
	}
	// The access token must be a real, verifiable JWT for the enrolled user.
	claims, err := auth.ParseJWT(testJWTSecret, body.AccessToken)
	if err != nil {
		t.Fatalf("access token not valid: %v", err)
	}
	if claims.Subject != "user-1" {
		t.Errorf("token subject = %q, want user-1", claims.Subject)
	}
	if fs.lastEnroll.Email != "friend@example.com" {
		t.Errorf("enroll request not passed through: %+v", fs.lastEnroll)
	}
}

func TestEnroll_InvalidInvite(t *testing.T) {
	fs := &fakeStore{enrollErr: store.ErrInviteInvalid}
	rr := httptest.NewRecorder()
	testServer(fs, authCfg()).
		ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/v1/enroll", strings.NewReader(validEnrollBody)))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rr.Code)
	}
}

func TestEnroll_DuplicateDeviceKey(t *testing.T) {
	fs := &fakeStore{enrollErr: store.ErrDeviceKeyTaken}
	rr := httptest.NewRecorder()
	testServer(fs, authCfg()).
		ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/v1/enroll", strings.NewReader(validEnrollBody)))
	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rr.Code)
	}
}

func TestEnroll_BadPlatform(t *testing.T) {
	body := `{"invite_code":"i","email":"a@b.co","device":{"name":"x","platform":"symbian","public_key":"k"}}`
	rr := httptest.NewRecorder()
	testServer(&fakeStore{}, authCfg()).
		ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/v1/enroll", strings.NewReader(body)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestRefresh_Success(t *testing.T) {
	fs := &fakeStore{refreshUserID: "user-9", refreshDeviceID: "device-9"}
	rr := httptest.NewRecorder()
	testServer(fs, authCfg()).
		ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/v1/auth/refresh",
			strings.NewReader(`{"refresh_token":"some-refresh"}`)))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var body tokenResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &body)
	claims, err := auth.ParseJWT(testJWTSecret, body.AccessToken)
	if err != nil || claims.Subject != "user-9" {
		t.Fatalf("new access token invalid or wrong subject: %v %q", err, claims.Subject)
	}
	if fs.lastRefreshTok != "some-refresh" {
		t.Errorf("refresh token not passed to store")
	}
}

func TestRefresh_Invalid(t *testing.T) {
	fs := &fakeStore{refreshErr: store.ErrInvalidToken}
	rr := httptest.NewRecorder()
	testServer(fs, authCfg()).
		ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/v1/auth/refresh",
			strings.NewReader(`{"refresh_token":"bad"}`)))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestDevices_RequireAuth(t *testing.T) {
	// No token.
	rr := httptest.NewRecorder()
	testServer(&fakeStore{}, authCfg()).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/devices", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("no-token status = %d, want 401", rr.Code)
	}
	// Auth disabled entirely.
	rr2 := httptest.NewRecorder()
	testServer(&fakeStore{}, ServerConfig{}).ServeHTTP(rr2, httptest.NewRequest(http.MethodGet, "/v1/devices", nil))
	if rr2.Code != http.StatusServiceUnavailable {
		t.Fatalf("disabled status = %d, want 503", rr2.Code)
	}
}

func TestDevices_ListForUser(t *testing.T) {
	fs := &fakeStore{devices: []model.Device{
		{ID: "d1", Name: "laptop", Platform: "macos", PublicKey: "K1", CreatedAt: time.Now()},
		{ID: "d2", Name: "phone", Platform: "android", PublicKey: "K2", CreatedAt: time.Now()},
	}}
	token, _ := auth.IssueJWT(testJWTSecret, "user-1", time.Hour)
	rr := httptest.NewRecorder()
	testServer(fs, authCfg()).ServeHTTP(rr, bearerReq(http.MethodGet, "/v1/devices", token, ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var body struct {
		Devices []deviceDTO `json:"devices"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &body)
	if len(body.Devices) != 2 || body.Devices[0].Name != "laptop" {
		t.Fatalf("unexpected devices: %+v", body.Devices)
	}
}

func TestDevices_Create(t *testing.T) {
	fs := &fakeStore{createDevice: model.Device{ID: "d3"}, createRefresh: "new-device-refresh"}
	token, _ := auth.IssueJWT(testJWTSecret, "user-1", time.Hour)
	body := `{"name":"tablet","platform":"ios","public_key":"K3"}`
	rr := httptest.NewRecorder()
	testServer(fs, authCfg()).ServeHTTP(rr, bearerReq(http.MethodPost, "/v1/devices", token, body))
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rr.Code)
	}
	var resp tokenResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.DeviceID != "d3" || resp.RefreshToken != "new-device-refresh" {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestDevices_Revoke(t *testing.T) {
	fs := &fakeStore{}
	token, _ := auth.IssueJWT(testJWTSecret, "user-7", time.Hour)
	rr := httptest.NewRecorder()
	testServer(fs, authCfg()).ServeHTTP(rr, bearerReq(http.MethodDelete, "/v1/devices/dev-42", token, ""))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rr.Code)
	}
	if fs.lastRevokeU != "user-7" || fs.lastRevokeD != "dev-42" {
		t.Errorf("revoke passed wrong ids: user=%q dev=%q", fs.lastRevokeU, fs.lastRevokeD)
	}

	// Not found.
	fs2 := &fakeStore{revokeErr: store.ErrNotFound}
	rr2 := httptest.NewRecorder()
	testServer(fs2, authCfg()).ServeHTTP(rr2, bearerReq(http.MethodDelete, "/v1/devices/nope", token, ""))
	if rr2.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr2.Code)
	}
}

func TestInviteCreate_AdminOnly(t *testing.T) {
	// Disabled when no admin token configured.
	rr := httptest.NewRecorder()
	testServer(&fakeStore{}, authCfg()).ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/v1/invites", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("disabled status = %d, want 404", rr.Code)
	}

	cfg := authCfg()
	cfg.AdminToken = testAdmin
	// Wrong token.
	rr2 := httptest.NewRecorder()
	testServer(&fakeStore{}, cfg).ServeHTTP(rr2, bearerReq(http.MethodPost, "/v1/invites", "nope", ""))
	if rr2.Code != http.StatusUnauthorized {
		t.Fatalf("wrong-token status = %d, want 401", rr2.Code)
	}
	// Success.
	fs := &fakeStore{}
	rr3 := httptest.NewRecorder()
	testServer(fs, cfg).ServeHTTP(rr3, bearerReq(http.MethodPost, "/v1/invites", testAdmin, `{"note":"for dana"}`))
	if rr3.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rr3.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(rr3.Body.Bytes(), &resp)
	if resp["code"] == nil || resp["code"] == "" {
		t.Errorf("invite code not returned")
	}
	if fs.lastInviteCode == "" {
		t.Errorf("invite code not passed to store")
	}
}

package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aevora/control-plane/internal/auth"
	"github.com/aevora/control-plane/internal/store"
)

func TestLogin_Success(t *testing.T) {
	hash, _ := auth.HashPassword("s3cret-password")
	fs := &fakeStore{credUserID: "user-1", credHash: hash, credStatus: "active"}
	rr := httptest.NewRecorder()
	testServer(fs, authCfg()).ServeHTTP(rr,
		bearerReq(http.MethodPost, "/v1/auth/login", "", `{"email":"a@b.co","password":"s3cret-password"}`))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rr.Code, rr.Body)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	hash, _ := auth.HashPassword("right")
	fs := &fakeStore{credUserID: "u", credHash: hash, credStatus: "active"}
	rr := httptest.NewRecorder()
	testServer(fs, authCfg()).ServeHTTP(rr,
		bearerReq(http.MethodPost, "/v1/auth/login", "", `{"email":"a@b.co","password":"wrong"}`))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestLogin_UnknownUserOrNoPassword(t *testing.T) {
	// Unknown user.
	fs := &fakeStore{credErr: store.ErrNotFound}
	rr := httptest.NewRecorder()
	testServer(fs, authCfg()).ServeHTTP(rr,
		bearerReq(http.MethodPost, "/v1/auth/login", "", `{"email":"x@y.z","password":"p"}`))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("unknown user status = %d, want 401", rr.Code)
	}
	// Known user, but no password set: same generic 401 (no enumeration).
	fs2 := &fakeStore{credUserID: "u", credHash: "", credStatus: "active"}
	rr2 := httptest.NewRecorder()
	testServer(fs2, authCfg()).ServeHTTP(rr2,
		bearerReq(http.MethodPost, "/v1/auth/login", "", `{"email":"a@b.co","password":"p"}`))
	if rr2.Code != http.StatusUnauthorized {
		t.Fatalf("no-password status = %d, want 401", rr2.Code)
	}
}

func TestSetPassword_RequiresAuthAndLength(t *testing.T) {
	// No token.
	rr := httptest.NewRecorder()
	testServer(&fakeStore{}, authCfg()).ServeHTTP(rr,
		httptest.NewRequest(http.MethodPost, "/v1/auth/password", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("no-auth status = %d, want 401", rr.Code)
	}
	// Authed but too short.
	token := userToken(t, "user-1")
	rr2 := httptest.NewRecorder()
	testServer(&fakeStore{}, authCfg()).ServeHTTP(rr2,
		bearerReq(http.MethodPost, "/v1/auth/password", token, `{"password":"short"}`))
	if rr2.Code != http.StatusBadRequest {
		t.Fatalf("short-password status = %d, want 400", rr2.Code)
	}
}

func TestSetPassword_Success(t *testing.T) {
	fs := &fakeStore{}
	token := userToken(t, "user-7")
	rr := httptest.NewRecorder()
	testServer(fs, authCfg()).ServeHTTP(rr,
		bearerReq(http.MethodPost, "/v1/auth/password", token, `{"password":"a-good-password"}`))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rr.Code)
	}
	if fs.lastSetPwdUser != "user-7" {
		t.Errorf("set password for wrong user: %s", fs.lastSetPwdUser)
	}
}

func TestRateLimit_LoginThrottled(t *testing.T) {
	cfg := authCfg()
	cfg.AuthRatePerMin = 1
	cfg.AuthBurst = 2
	fs := &fakeStore{credErr: store.ErrNotFound}
	srv := testServer(fs, cfg)

	codes := make([]int, 0, 4)
	for i := 0; i < 4; i++ {
		rr := httptest.NewRecorder()
		req := bearerReq(http.MethodPost, "/v1/auth/login", "", `{"email":"a@b.co","password":"p"}`)
		req.RemoteAddr = "203.0.113.5:12345" // same client IP
		srv.ServeHTTP(rr, req)
		codes = append(codes, rr.Code)
	}
	// With burst 2, the first two pass (401 invalid creds), later ones are 429.
	if codes[0] == http.StatusTooManyRequests || codes[1] == http.StatusTooManyRequests {
		t.Fatalf("first requests should not be throttled: %v", codes)
	}
	if codes[3] != http.StatusTooManyRequests {
		t.Fatalf("later requests should be throttled: %v", codes)
	}
}

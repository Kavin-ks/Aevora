package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aevora/control-plane/internal/auth"
	"github.com/aevora/control-plane/internal/model"
	"github.com/aevora/control-plane/internal/store"
)

func userToken(t *testing.T, uid string) string {
	t.Helper()
	tok, err := auth.IssueJWT(testJWTSecret, uid, time.Hour)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return tok
}

func connCfg() ServerConfig {
	c := authCfg()
	c.LeaseTTL = 5 * time.Minute
	c.HeartbeatTTL = 30 * time.Second
	c.ClientDNS = []string{"9.9.9.9"}
	return c
}

func TestConnectionCreate_Success(t *testing.T) {
	v6 := "fd07:0007:1::5/128"
	fs := &fakeStore{connResult: model.Connection{
		ID: "lease-1", GatewayName: "de-fra-1", Country: "Germany", City: "Frankfurt",
		Endpoint: "203.0.113.1:51820", GatewayPublicKey: "GWPUB=",
		AssignedIPv4: "10.7.1.5/32", AssignedIPv6: &v6, ExpiresAt: time.Now().Add(5 * time.Minute),
	}}
	token := userToken(t, "user-1")
	rr := httptest.NewRecorder()
	testServer(fs, connCfg()).ServeHTTP(rr,
		bearerReq(http.MethodPost, "/v1/connections", token, `{"country_code":"de","device_id":"dev-1"}`))

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (%s)", rr.Code, rr.Body)
	}
	var resp connectionResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Server.Endpoint != "203.0.113.1:51820" || resp.Server.PublicKey != "GWPUB=" {
		t.Errorf("server info wrong: %+v", resp.Server)
	}
	if resp.AssignedIP != "10.7.1.5/32" || resp.AssignedIPv6 == nil {
		t.Errorf("assigned IPs wrong: %s %v", resp.AssignedIP, resp.AssignedIPv6)
	}
	if len(resp.AllowedIPs) != 2 || resp.AllowedIPs[0] != "0.0.0.0/0" {
		t.Errorf("allowed_ips wrong: %v", resp.AllowedIPs)
	}
	if fs.lastConnUser != "user-1" || fs.lastConnDevice != "dev-1" || fs.lastConnCountry != "de" {
		t.Errorf("args not passed through: %s %s %s", fs.lastConnUser, fs.lastConnDevice, fs.lastConnCountry)
	}
}

func TestConnectionCreate_RequiresAuth(t *testing.T) {
	rr := httptest.NewRecorder()
	testServer(&fakeStore{}, connCfg()).ServeHTTP(rr,
		httptest.NewRequest(http.MethodPost, "/v1/connections", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestConnectionCreate_MissingFields(t *testing.T) {
	token := userToken(t, "u")
	rr := httptest.NewRecorder()
	testServer(&fakeStore{}, connCfg()).ServeHTTP(rr,
		bearerReq(http.MethodPost, "/v1/connections", token, `{"country_code":"de"}`))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestConnectionCreate_DeviceNotOwned(t *testing.T) {
	fs := &fakeStore{connErr: store.ErrDeviceNotFound}
	token := userToken(t, "u")
	rr := httptest.NewRecorder()
	testServer(fs, connCfg()).ServeHTTP(rr,
		bearerReq(http.MethodPost, "/v1/connections", token, `{"country_code":"de","device_id":"x"}`))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func TestConnectionCreate_NoGateway(t *testing.T) {
	fs := &fakeStore{connErr: store.ErrNoGatewayAvailable}
	token := userToken(t, "u")
	rr := httptest.NewRecorder()
	testServer(fs, connCfg()).ServeHTTP(rr,
		bearerReq(http.MethodPost, "/v1/connections", token, `{"country_code":"xx","device_id":"d"}`))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rr.Code)
	}
}

func TestConnectionDelete(t *testing.T) {
	fs := &fakeStore{}
	token := userToken(t, "user-7")
	rr := httptest.NewRecorder()
	testServer(fs, connCfg()).ServeHTTP(rr, bearerReq(http.MethodDelete, "/v1/connections/lease-9", token, ""))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rr.Code)
	}
	if fs.lastReleaseUser != "user-7" || fs.lastReleaseLease != "lease-9" {
		t.Errorf("release args wrong: %s %s", fs.lastReleaseUser, fs.lastReleaseLease)
	}

	fs2 := &fakeStore{releaseErr: store.ErrNotFound}
	rr2 := httptest.NewRecorder()
	testServer(fs2, connCfg()).ServeHTTP(rr2, bearerReq(http.MethodDelete, "/v1/connections/nope", token, ""))
	if rr2.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr2.Code)
	}
}

func TestConnectionStats_RenewsLease(t *testing.T) {
	exp := time.Now().Add(5 * time.Minute)
	fs := &fakeStore{renewExpiry: exp}
	token := userToken(t, "u")
	rr := httptest.NewRecorder()
	testServer(fs, connCfg()).ServeHTTP(rr,
		bearerReq(http.MethodPost, "/v1/connections/lease-1/stats", token, `{"rx_bps":100,"tx_bps":200}`))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &body)
	if body["expires_at"] == nil {
		t.Errorf("expires_at not returned")
	}

	fs2 := &fakeStore{renewErr: store.ErrNotFound}
	rr2 := httptest.NewRecorder()
	testServer(fs2, connCfg()).ServeHTTP(rr2,
		bearerReq(http.MethodPost, "/v1/connections/nope/stats", token, `{}`))
	if rr2.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr2.Code)
	}
}

func TestGatewayPeers_Success(t *testing.T) {
	fs := &fakeStore{peers: []model.Peer{
		{PublicKey: "CK1", AllowedIPs: []string{"10.7.1.5/32", "fd07:0007:1::5/128"}},
		{PublicKey: "CK2", AllowedIPs: []string{"10.7.1.6/32"}},
	}}
	token := "node-token-xyz"
	rr := httptest.NewRecorder()
	testServer(fs, connCfg()).ServeHTTP(rr, bearerReq(http.MethodGet, "/v1/gateways/peers", token, ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if fs.lastPeersHash != auth.HashToken(token) {
		t.Errorf("peers must be looked up by token hash")
	}
	var body struct {
		Peers []model.Peer `json:"peers"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Peers) != 2 || body.Peers[0].PublicKey != "CK1" || len(body.Peers[0].AllowedIPs) != 2 {
		t.Fatalf("unexpected peers: %+v", body.Peers)
	}
}

func TestGatewayPeers_InvalidToken(t *testing.T) {
	fs := &fakeStore{peersErr: store.ErrInvalidToken}
	rr := httptest.NewRecorder()
	testServer(fs, connCfg()).ServeHTTP(rr, bearerReq(http.MethodGet, "/v1/gateways/peers", "bad", ""))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
	// Missing token.
	rr2 := httptest.NewRecorder()
	testServer(&fakeStore{}, connCfg()).ServeHTTP(rr2, httptest.NewRequest(http.MethodGet, "/v1/gateways/peers", nil))
	if rr2.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr2.Code)
	}
}

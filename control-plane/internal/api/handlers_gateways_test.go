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

const (
	testEnroll = "enroll-secret-xyz"
	testAdmin  = "admin-token-xyz"
)

func post(h http.Handler, path, bearer, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

const validRegBody = `{
  "name":"de-fra-9","country_code":"de","country_name":"Germany","city":"Frankfurt",
  "region":"eu-central","endpoint_host":"203.0.113.9","public_key":"PUBKEY_FRA9=",
  "wg_subnet_v4":"10.7.9.0/24","capacity":250
}`

func TestRegister_Success(t *testing.T) {
	fs := &fakeStore{
		registerGW:    model.Gateway{ID: "gw-1", Name: "de-fra-9", Status: model.GatewayPending},
		registerToken: "one-time-node-token",
	}
	h := testServer(fs, ServerConfig{EnrollmentSecret: testEnroll})
	rr := post(h, "/v1/gateways/register", testEnroll, validRegBody)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (%s)", rr.Code, rr.Body)
	}
	var body map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &body)
	if body["node_token"] != "one-time-node-token" {
		t.Errorf("node_token not returned, got %v", body["node_token"])
	}
	// endpoint_port omitted in body -> handler defaults it to 51820 before store
	if fs.lastReg.EndpointPort != 51820 {
		t.Errorf("default port not applied, got %d", fs.lastReg.EndpointPort)
	}
	if fs.lastReg.CountryCode != "de" {
		t.Errorf("registration not passed through, got %+v", fs.lastReg)
	}
}

func TestRegister_DisabledWhenNoSecret(t *testing.T) {
	rr := post(testServer(&fakeStore{}, ServerConfig{}), "/v1/gateways/register", "anything", validRegBody)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rr.Code)
	}
}

func TestRegister_WrongSecret(t *testing.T) {
	h := testServer(&fakeStore{}, ServerConfig{EnrollmentSecret: testEnroll})
	rr := post(h, "/v1/gateways/register", "wrong", validRegBody)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestRegister_InvalidBody(t *testing.T) {
	h := testServer(&fakeStore{}, ServerConfig{EnrollmentSecret: testEnroll})
	rr := post(h, "/v1/gateways/register", testEnroll, `{"country_code":"de"}`) // missing name etc.
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestHeartbeat_Success(t *testing.T) {
	fs := &fakeStore{heartbeatGW: model.Gateway{ID: "gw-1", Status: model.GatewayHealthy}}
	h := testServer(fs, ServerConfig{})
	token := "node-token-abc"
	rr := post(h, "/v1/gateways/heartbeat", token, `{"active_peers":42,"cpu_pct":12.5,"rx_bps":1000,"tx_bps":2000}`)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if fs.lastHBHash != auth.HashToken(token) {
		t.Errorf("handler must pass the token HASH, not the token")
	}
	if fs.lastHBMetrics.ActivePeers != 42 || fs.lastHBMetrics.TxBps != 2000 {
		t.Errorf("metrics not passed through: %+v", fs.lastHBMetrics)
	}
}

func TestHeartbeat_InvalidToken(t *testing.T) {
	fs := &fakeStore{heartbeatErr: store.ErrInvalidToken}
	rr := post(testServer(fs, ServerConfig{}), "/v1/gateways/heartbeat", "bad", `{}`)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestHeartbeat_MissingToken(t *testing.T) {
	rr := post(testServer(&fakeStore{}, ServerConfig{}), "/v1/gateways/heartbeat", "", `{}`)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestDeregister(t *testing.T) {
	fs := &fakeStore{}
	rr := post(testServer(fs, ServerConfig{}), "/v1/gateways/deregister", "node-token-abc", ``)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rr.Code)
	}
	if fs.lastDeregHT != auth.HashToken("node-token-abc") {
		t.Errorf("deregister must use the token hash")
	}

	fs2 := &fakeStore{deregErr: store.ErrInvalidToken}
	rr2 := post(testServer(fs2, ServerConfig{}), "/v1/gateways/deregister", "bad", ``)
	if rr2.Code != http.StatusUnauthorized {
		t.Fatalf("invalid token status = %d, want 401", rr2.Code)
	}
}

func TestGatewayList_OnlineDerivedFromTTL(t *testing.T) {
	now := time.Now()
	fresh := now.Add(-5 * time.Second)
	stale := now.Add(-5 * time.Minute)
	fs := &fakeStore{gateways: []model.Gateway{
		{ID: "1", Name: "de-fra-1", CountryName: "Germany", LocationCode: "de",
			EndpointHost: "203.0.113.1", EndpointPort: 51820,
			Status: model.GatewayHealthy, LastHeartbeatAt: &fresh}, // online
		{ID: "2", Name: "de-fra-2", CountryName: "Germany", LocationCode: "de",
			EndpointHost: "203.0.113.2", EndpointPort: 51820,
			Status: model.GatewayHealthy, LastHeartbeatAt: &stale}, // stale -> offline
		{ID: "3", Name: "sg-sin-1", CountryName: "Singapore", LocationCode: "sg",
			EndpointHost: "203.0.113.3", EndpointPort: 51820,
			Status: model.GatewayHealthy, LastHeartbeatAt: nil}, // never -> offline
	}}
	h := testServer(fs, ServerConfig{AdminToken: testAdmin, HeartbeatTTL: 30 * time.Second})

	req := httptest.NewRequest(http.MethodGet, "/v1/gateways", nil)
	req.Header.Set("Authorization", "Bearer "+testAdmin)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var body struct {
		Gateways []gatewayDTO `json:"gateways"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Gateways) != 3 {
		t.Fatalf("got %d gateways, want 3", len(body.Gateways))
	}
	if !body.Gateways[0].Online {
		t.Error("fresh heartbeat should be online")
	}
	if body.Gateways[1].Online {
		t.Error("stale heartbeat should be offline")
	}
	if body.Gateways[2].Online {
		t.Error("never-heartbeated should be offline")
	}
	if body.Gateways[0].Endpoint != "203.0.113.1:51820" {
		t.Errorf("endpoint = %q", body.Gateways[0].Endpoint)
	}
}

func TestGatewayList_DisabledWhenNoAdminToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/gateways", nil)
	rr := httptest.NewRecorder()
	testServer(&fakeStore{}, ServerConfig{}).ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (feature off)", rr.Code)
	}
}

func TestGatewayList_WrongAdminToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/gateways", nil)
	req.Header.Set("Authorization", "Bearer nope")
	rr := httptest.NewRecorder()
	testServer(&fakeStore{}, ServerConfig{AdminToken: testAdmin}).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

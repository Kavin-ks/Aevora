package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aevora/control-plane/internal/model"
)

// fakeStore implements api.Store without a database. Exported-ish fields let
// tests seed return values and inspect what handlers passed down.
type fakeStore struct {
	locs   []model.Location
	locErr error

	registerGW    model.Gateway
	registerToken string
	registerErr   error
	lastReg       model.GatewayRegistration

	heartbeatGW   model.Gateway
	heartbeatErr  error
	lastHBHash    string
	lastHBMetrics model.GatewayMetrics

	deregErr    error
	lastDeregHT string

	gateways []model.Gateway
	listErr  error

	// identity
	enrollUser    model.User
	enrollDevice  model.Device
	enrollRefresh string
	enrollErr     error
	lastEnroll    model.EnrollRequest

	createDevice  model.Device
	createRefresh string
	createErr     error

	devices    []model.Device
	listDevErr error

	revokeErr   error
	lastRevokeU string
	lastRevokeD string

	refreshUserID   string
	refreshDeviceID string
	refreshErr      error
	lastRefreshTok  string

	inviteErr      error
	lastInviteCode string

	// connections
	connResult      model.Connection
	connErr         error
	lastConnUser    string
	lastConnDevice  string
	lastConnCountry string

	releaseErr       error
	lastReleaseUser  string
	lastReleaseLease string

	renewExpiry time.Time
	renewErr    error

	peers         []model.Peer
	peersErr      error
	lastPeersHash string
}

func (f *fakeStore) CreateConnection(_ context.Context, userID, deviceID, country string, _, _ time.Duration) (model.Connection, error) {
	f.lastConnUser, f.lastConnDevice, f.lastConnCountry = userID, deviceID, country
	return f.connResult, f.connErr
}
func (f *fakeStore) ReleaseConnection(_ context.Context, userID, leaseID string) error {
	f.lastReleaseUser, f.lastReleaseLease = userID, leaseID
	return f.releaseErr
}
func (f *fakeStore) RenewConnection(_ context.Context, _, _ string, _ time.Duration) (time.Time, error) {
	return f.renewExpiry, f.renewErr
}
func (f *fakeStore) GatewayPeers(_ context.Context, tokenHash string) ([]model.Peer, error) {
	f.lastPeersHash = tokenHash
	return f.peers, f.peersErr
}

func (f *fakeStore) Enroll(_ context.Context, r model.EnrollRequest, _ time.Duration) (model.User, model.Device, string, error) {
	f.lastEnroll = r
	return f.enrollUser, f.enrollDevice, f.enrollRefresh, f.enrollErr
}
func (f *fakeStore) CreateDevice(_ context.Context, _ string, _ model.DeviceRegistration, _ time.Duration) (model.Device, string, error) {
	return f.createDevice, f.createRefresh, f.createErr
}
func (f *fakeStore) ListDevices(context.Context, string) ([]model.Device, error) {
	return f.devices, f.listDevErr
}
func (f *fakeStore) RevokeDevice(_ context.Context, userID, deviceID string) error {
	f.lastRevokeU, f.lastRevokeD = userID, deviceID
	return f.revokeErr
}
func (f *fakeStore) RefreshAccess(_ context.Context, plain string) (string, string, error) {
	f.lastRefreshTok = plain
	return f.refreshUserID, f.refreshDeviceID, f.refreshErr
}
func (f *fakeStore) CreateInvite(_ context.Context, code, _ string, _ *time.Time) error {
	f.lastInviteCode = code
	return f.inviteErr
}

func (f *fakeStore) ListLocations(context.Context) ([]model.Location, error) {
	return f.locs, f.locErr
}
func (f *fakeStore) RegisterGateway(_ context.Context, r model.GatewayRegistration) (model.Gateway, string, error) {
	f.lastReg = r
	return f.registerGW, f.registerToken, f.registerErr
}
func (f *fakeStore) Heartbeat(_ context.Context, tokenHash string, m model.GatewayMetrics) (model.Gateway, error) {
	f.lastHBHash, f.lastHBMetrics = tokenHash, m
	return f.heartbeatGW, f.heartbeatErr
}
func (f *fakeStore) DeregisterGateway(_ context.Context, tokenHash string) error {
	f.lastDeregHT = tokenHash
	return f.deregErr
}
func (f *fakeStore) ListGateways(context.Context) ([]model.Gateway, error) {
	return f.gateways, f.listErr
}

func testServer(fs *fakeStore, cfg ServerConfig) http.Handler {
	return NewServer(fs, cfg, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler()
}

func TestHealth(t *testing.T) {
	rr := httptest.NewRecorder()
	testServer(&fakeStore{}, ServerConfig{}).
		ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
}

func TestLocations_FlattensAvailability(t *testing.T) {
	fs := &fakeStore{locs: []model.Location{
		{Code: "de", CountryName: "Germany", ServerCount: 2, HealthyCount: 2},
		{Code: "us", CountryName: "United States", ServerCount: 1, HealthyCount: 0},
	}}
	rr := httptest.NewRecorder()
	testServer(fs, ServerConfig{}).
		ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/locations", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var body struct {
		Locations []locationDTO `json:"locations"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Locations) != 2 {
		t.Fatalf("got %d locations, want 2", len(body.Locations))
	}
	if !body.Locations[0].Available {
		t.Error("Germany should be available")
	}
	if body.Locations[1].Available {
		t.Error("United States should be unavailable (no healthy gateway)")
	}
}

func TestLocations_StoreErrorIs500(t *testing.T) {
	rr := httptest.NewRecorder()
	testServer(&fakeStore{locErr: errors.New("db down")}, ServerConfig{}).
		ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/locations", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rr.Code)
	}
}

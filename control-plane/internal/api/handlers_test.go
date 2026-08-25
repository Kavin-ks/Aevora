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

	"github.com/aevora/control-plane/internal/model"
)

// fakeLocations implements LocationLister without a database.
type fakeLocations struct {
	locs []model.Location
	err  error
}

func (f fakeLocations) ListLocations(context.Context) ([]model.Location, error) {
	return f.locs, f.err
}

func newTestServer(fl fakeLocations) http.Handler {
	return NewServer(fl, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler()
}

func TestHealth(t *testing.T) {
	rr := httptest.NewRecorder()
	newTestServer(fakeLocations{}).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
}

func TestLocations_FlattensAvailability(t *testing.T) {
	fl := fakeLocations{locs: []model.Location{
		{Code: "de", CountryName: "Germany", ServerCount: 2, HealthyCount: 2},
		{Code: "us", CountryName: "United States", ServerCount: 1, HealthyCount: 0}, // unavailable
	}}
	rr := httptest.NewRecorder()
	newTestServer(fl).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/locations", nil))
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
		t.Errorf("Germany should be available")
	}
	if body.Locations[1].Available {
		t.Errorf("United States should be unavailable (no healthy gateway)")
	}
}

func TestLocations_StoreErrorIs500(t *testing.T) {
	rr := httptest.NewRecorder()
	newTestServer(fakeLocations{err: errors.New("db down")}).
		ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/locations", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rr.Code)
	}
}

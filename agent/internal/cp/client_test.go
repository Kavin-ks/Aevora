package cp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegister_ReturnsToken(t *testing.T) {
	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"gw-1","node_token":"the-node-token"}`))
	}))
	defer srv.Close()

	tok, err := New(srv.URL).Register(context.Background(), "enroll-secret", Registration{Name: "de-fra-1"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if tok != "the-node-token" {
		t.Fatalf("token = %q", tok)
	}
	if gotAuth != "Bearer enroll-secret" {
		t.Errorf("auth header = %q", gotAuth)
	}
	if gotPath != "/v1/gateways/register" {
		t.Errorf("path = %q", gotPath)
	}
}

func TestPeers_ParsesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer node-tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"peers":[
			{"public_key":"CK1","allowed_ips":["10.7.0.2/32","fd07::2/128"]},
			{"public_key":"CK2","allowed_ips":["10.7.0.3/32"]}
		]}`))
	}))
	defer srv.Close()

	peers, err := New(srv.URL).Peers(context.Background(), "node-tok")
	if err != nil {
		t.Fatalf("peers: %v", err)
	}
	if len(peers) != 2 || peers[0].PublicKey != "CK1" || len(peers[0].AllowedIPs) != 2 {
		t.Fatalf("unexpected peers: %+v", peers)
	}
}

func TestDo_Non2xxIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"invalid node token"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	if err := New(srv.URL).Heartbeat(context.Background(), "bad", Metrics{}); err == nil {
		t.Fatal("expected error on 401")
	}
}

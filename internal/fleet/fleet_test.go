package fleet_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"

	"github.com/hermes-scheduler/hermes/internal/api"
	"github.com/hermes-scheduler/hermes/internal/auth"
	"github.com/hermes-scheduler/hermes/internal/config"
	"github.com/hermes-scheduler/hermes/internal/fleet"
	"github.com/hermes-scheduler/hermes/internal/notifier"
	"github.com/hermes-scheduler/hermes/internal/testutil"
)

func setupNode(t *testing.T, nodeID string) (*httptest.Server, *fleet.Manager, string) {
	t.Helper()
	db := testutil.TestDB(t)
	domainURL := "http://" + nodeID + ".test"
	notif := notifier.New(db, &config.NotifyConfig{}, domainURL, nodeID)
	mgr, err := fleet.New(db, notif, config.ServerConfig{DomainURL: domainURL, ServerName: nodeID}, config.FleetConfig{NodeID: nodeID})
	if err != nil {
		t.Fatalf("fleet.New: %v", err)
	}

	apiHandler := api.New(db, nil, nil, mgr)
	router := mux.NewRouter()
	apiRouter := router.PathPrefix("/api").Subrouter()
	apiHandler.RegisterFleetPublicRoutes(apiRouter)
	secret, _ := mgr.PeerSecret()
	apiRouter.Handle("/fleet/handshake", auth.PeerAuthMiddleware(func() string { return secret })(http.HandlerFunc(apiHandler.FleetHandshake))).Methods("POST")
	apiRouter.Handle("/fleet/heartbeat", auth.PeerAuthMiddleware(func() string { return secret })(http.HandlerFunc(apiHandler.FleetHeartbeat))).Methods("POST")

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	// Point domain at test server URL for handshake callbacks
	settings, _ := mgr.LocalSettings()
	settings.Name = nodeID
	_ = db.SaveNodeSettings(settings)

	return server, mgr, secret
}

func TestFleetStandaloneNoPeers(t *testing.T) {
	_, mgr, _ := setupNode(t, "solo")
	resp, err := mgr.ListPeersResponse()
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Peers) != 0 {
		t.Fatalf("expected 0 peers, got %d", len(resp.Peers))
	}
}

func TestFleetTwoNodeHandshake(t *testing.T) {
	nodeB, mgrB, secretB := setupNode(t, "node-b")
	_, mgrA, _ := setupNode(t, "node-a")

	peer, err := mgrA.AddPeer("Node B", nodeB.URL, secretB)
	if err != nil {
		t.Fatalf("AddPeer: %v", err)
	}
	if peer.NodeID != "node-b" {
		t.Fatalf("peer node_id=%s", peer.NodeID)
	}

	respA, _ := mgrA.ListPeersResponse()
	if len(respA.Peers) != 1 {
		t.Fatalf("node A peers=%d", len(respA.Peers))
	}

	respB, _ := mgrB.ListPeersResponse()
	if len(respB.Peers) != 1 {
		t.Fatalf("node B should have node A registered, peers=%d", len(respB.Peers))
	}
	if respB.Peers[0].NodeID != "node-a" {
		t.Fatalf("node B peer id=%s", respB.Peers[0].NodeID)
	}
}

func TestNormalizeAddress(t *testing.T) {
	tests := map[string]string{
		"host:4376":              "http://host:4376",
		"http://host:4376/":      "http://host:4376",
		"https://hermes.example": "https://hermes.example",
	}
	for in, want := range tests {
		if got := fleet.NormalizeAddress(in); got != want {
			t.Fatalf("%q => %q, want %q", in, got, want)
		}
	}
}

func TestValidatePeerURLRejectsNonHTTP(t *testing.T) {
	_, mgr, _ := setupNode(t, "solo")
	_, err := mgr.AddPeer("bad", "file:///etc/passwd", "abcd")
	if err == nil {
		t.Fatal("expected error for file:// URL")
	}
}

func TestPeerAuthMiddleware(t *testing.T) {
	called := false
	handler := auth.PeerAuthMiddleware(func() string { return "secret-token" })(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodPost, "/fleet/handshake", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !called {
		t.Fatalf("expected authorized request, code=%d called=%v", rr.Code, called)
	}

	req.Header.Set("Authorization", "Bearer wrong")
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

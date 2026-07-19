package fleet_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"

	"github.com/hermes-scheduler/hermes/internal/api"
	"github.com/hermes-scheduler/hermes/internal/auth"
	"github.com/hermes-scheduler/hermes/internal/config"
	"github.com/hermes-scheduler/hermes/internal/fleet"
	"github.com/hermes-scheduler/hermes/internal/models"
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
	apiRouter.Handle("/fleet/heartbeat", auth.PeerHeartbeatMiddleware(mgr.AuthenticatePeerSecret)(http.HandlerFunc(apiHandler.FleetHeartbeat))).Methods("POST")

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

func TestHandleHandshakeRejectsLocalhostCallback(t *testing.T) {
	_, mgr, _ := setupNode(t, "solo")
	_, err := mgr.HandleHandshake(models.FleetPeerPayload{
		NodeID:     "remote",
		Name:       "Remote",
		Address:    "http://localhost:4376",
		PeerSecret: "secret",
	})
	if err == nil {
		t.Fatal("expected error for localhost callback URL")
	}
}

func TestHandleHeartbeatAndAuthenticate(t *testing.T) {
	nodeB, mgrB, secretB := setupNode(t, "node-b")
	_, mgrA, secretA := setupNode(t, "node-a")

	if _, err := mgrA.AddPeer("Node B", nodeB.URL, secretB); err != nil {
		t.Fatalf("AddPeer: %v", err)
	}

	if !mgrB.AuthenticatePeerSecret(secretA) {
		t.Fatal("expected peer secret to authenticate")
	}
	if mgrB.AuthenticatePeerSecret("wrong") {
		t.Fatal("unexpected auth for wrong secret")
	}

	err := mgrB.HandleHeartbeat(secretA, models.FleetPeerPayload{
		NodeID:  "node-a",
		Name:    "Node A",
		Address: "http://node-a.test",
	})
	if err != nil {
		t.Fatalf("HandleHeartbeat: %v", err)
	}

	resp, _ := mgrB.ListPeersResponse()
	if len(resp.Peers) != 1 || resp.Peers[0].Status != models.PeerStatusOnline {
		t.Fatalf("peer=%+v", resp.Peers)
	}
}

func TestHandleHeartbeatRejectsNodeIDMismatch(t *testing.T) {
	_, mgr, _ := setupNode(t, "solo")
	_, err := mgr.HandleHandshake(models.FleetPeerPayload{
		NodeID: "remote", Name: "Remote", Address: "http://remote.test:4376", PeerSecret: "remote-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	err = mgr.HandleHeartbeat("remote-secret", models.FleetPeerPayload{NodeID: "other-id"})
	if err == nil {
		t.Fatal("expected node_id mismatch error")
	}
}

func TestRemovePeerAndUpdateLocal(t *testing.T) {
	_, mgr, _ := setupNode(t, "solo")
	_, err := mgr.HandleHandshake(models.FleetPeerPayload{
		NodeID: "remote", Name: "Remote", Address: "http://remote.test:4376", PeerSecret: "remote-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	peers, _ := mgr.ListPeersResponse()
	if len(peers.Peers) != 1 {
		t.Fatal("expected peer")
	}

	if err := mgr.RemovePeer(peers.Peers[0].ID); err != nil {
		t.Fatalf("RemovePeer: %v", err)
	}
	resp, _ := mgr.ListPeersResponse()
	if len(resp.Peers) != 0 {
		t.Fatalf("peers=%d", len(resp.Peers))
	}

	settings, err := mgr.UpdateLocal("New Name", true)
	if err != nil {
		t.Fatalf("UpdateLocal: %v", err)
	}
	if settings.Name != "New Name" {
		t.Fatalf("name=%q", settings.Name)
	}
}

func TestCheckPeersOutboundHeartbeat(t *testing.T) {
	nodeB, _, secretB := setupNode(t, "node-b")
	_, mgrA, _ := setupNode(t, "node-a")

	if _, err := mgrA.AddPeer("Node B", nodeB.URL, secretB); err != nil {
		t.Fatalf("AddPeer: %v", err)
	}

	mgrA.Start()
	time.Sleep(200 * time.Millisecond)
	mgrA.Stop()

	resp, _ := mgrA.ListPeersResponse()
	if len(resp.Peers) != 1 {
		t.Fatalf("peers=%d", len(resp.Peers))
	}
	if resp.Peers[0].Status != models.PeerStatusOnline {
		t.Fatalf("status=%s", resp.Peers[0].Status)
	}
}

func TestDefaultNodeIDUnique(t *testing.T) {
	db := testutil.TestDB(t)
	notif := notifier.New(db, &config.NotifyConfig{}, "http://localhost:4376", "test")
	mgr, err := fleet.New(db, notif, config.ServerConfig{ServerName: "shared-host"}, config.FleetConfig{})
	if err != nil {
		t.Fatal(err)
	}
	settings, err := mgr.LocalSettings()
	if err != nil || settings == nil {
		t.Fatal(err)
	}
	if !strings.Contains(settings.NodeID, "shared-host") {
		t.Fatalf("node_id=%q", settings.NodeID)
	}
	if !strings.Contains(settings.NodeID, "-") {
		t.Fatalf("expected random suffix in node_id=%q", settings.NodeID)
	}
}

func TestLocalNodeInfoAndPeerSecret(t *testing.T) {
	_, mgr, secret := setupNode(t, "info-node")
	info, err := mgr.LocalNodeInfo()
	if err != nil {
		t.Fatal(err)
	}
	if info.NodeID != "info-node" || info.Address == "" {
		t.Fatalf("info=%+v", info)
	}
	got, err := mgr.PeerSecret()
	if err != nil || got != secret {
		t.Fatalf("secret=%q err=%v", got, err)
	}
}

func TestAddPeerRequiresDomainURL(t *testing.T) {
	db := testutil.TestDB(t)
	notif := notifier.New(db, &config.NotifyConfig{}, "", "test")
	mgr, err := fleet.New(db, notif, config.ServerConfig{}, config.FleetConfig{NodeID: "solo"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = mgr.AddPeer("X", "http://x.test:4376", "secret")
	if err == nil || !strings.Contains(err.Error(), "HERMES_DOMAIN_URL") {
		t.Fatalf("err=%v", err)
	}
}

func TestUpsertPeerIdempotentByNodeID(t *testing.T) {
	nodeB, _, secretB := setupNode(t, "node-b")
	_, mgrA, _ := setupNode(t, "node-a")

	if _, err := mgrA.AddPeer("Node B", nodeB.URL, secretB); err != nil {
		t.Fatal(err)
	}
	if _, err := mgrA.AddPeer("Node B Again", nodeB.URL, secretB); err != nil {
		t.Fatalf("retry AddPeer: %v", err)
	}
	resp, _ := mgrA.ListPeersResponse()
	if len(resp.Peers) != 1 {
		t.Fatalf("peers=%d", len(resp.Peers))
	}
}

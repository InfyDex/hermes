package fleet

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hermes-scheduler/hermes/internal/config"
	"github.com/hermes-scheduler/hermes/internal/database"
	"github.com/hermes-scheduler/hermes/internal/models"
	"github.com/hermes-scheduler/hermes/internal/notifier"
)

func testManager(t *testing.T) *Manager {
	t.Helper()
	db, err := database.New(filepath.Join(t.TempDir(), "fleet.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	notif := notifier.New(db, &config.NotifyConfig{}, "http://localhost:4376", "test")
	mgr, err := New(db, notif, config.ServerConfig{DomainURL: "http://localhost:4376"}, config.FleetConfig{NodeID: "test-node"})
	if err != nil {
		t.Fatal(err)
	}
	return mgr
}

func TestMaybeNotifyTransitionLocked(t *testing.T) {
	mgr := testManager(t)

	mgr.mu.Lock()
	mgr.maybeNotifyTransitionLocked(1, "Peer", models.PeerStatusOnline, models.PeerStatusOffline)
	mgr.maybeNotifyTransitionLocked(1, "Peer", models.PeerStatusOffline, models.PeerStatusOnline)
	mgr.maybeNotifyTransitionLocked(1, "Peer", models.PeerStatusOnline, models.PeerStatusOnline)
	mgr.mu.Unlock()
}

func TestPingPeerFailureAndSuccess(t *testing.T) {
	mgr := testManager(t)
	local, _ := mgr.LocalSettings()
	now := time.Now().UTC()

	dead := models.Peer{
		ID: 1, NodeID: "dead", Name: "Dead", Address: "http://127.0.0.1:1",
		Status: models.PeerStatusOnline, LastSeenAt: &now,
	}
	payload := models.FleetPeerPayload{NodeID: local.NodeID, Name: local.Name, Address: mgr.callbackAddress()}

	for i := 0; i < 3; i++ {
		mgr.pingPeer(dead, local, payload)
	}

	okServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(okServer.Close)

	alive := models.Peer{
		ID: 2, NodeID: "alive", Name: "Alive", Address: okServer.URL,
		Status: models.PeerStatusOffline,
	}
	mgr.pingPeer(alive, local, payload)
}

func TestSetPeerStatusLockedRefreshLastSeen(t *testing.T) {
	mgr := testManager(t)
	db := mgr.db
	now := time.Now().UTC()
	peer := &models.Peer{
		NodeID: "p1", Name: "P1", Address: "http://p1.test", PeerSecret: "s",
		Status: models.PeerStatusOnline, LastSeenAt: &now,
	}
	if err := db.UpsertPeer(peer); err != nil {
		t.Fatal(err)
	}

	later := now.Add(time.Minute)
	mgr.mu.Lock()
	mgr.setPeerStatusLocked(*peer, models.PeerStatusOnline, &later)
	mgr.mu.Unlock()
}

func TestValidatePeerCallbackURLBlockedHosts(t *testing.T) {
	cases := []string{
		"http://localhost:4376",
		"http://127.0.0.1:4376",
		"http://169.254.169.254/",
		"http://metadata.google.internal/",
	}
	for _, addr := range cases {
		if err := validatePeerCallbackURL(addr); err == nil {
			t.Fatalf("expected block for %s", addr)
		}
	}
	if err := validatePeerCallbackURL("http://10.0.0.5:4376"); err != nil {
		t.Fatalf("homelab private IP should be allowed: %v", err)
	}
}

func TestNormalizeAddressAndAddressJoin(t *testing.T) {
	if got := NormalizeAddress(""); got != "" {
		t.Fatalf("empty = %q", got)
	}
	if got := addressJoin("http://host", "/api"); got != "http://host/api" {
		t.Fatalf("join = %q", got)
	}
}

func TestPeerHostAndSanitizeID(t *testing.T) {
	if got := peerHost("http://Example.COM:4376/path"); got != "example.com" {
		t.Fatalf("host = %q", got)
	}
	if got := sanitizeID("My Server"); got != "my-server" {
		t.Fatalf("sanitize = %q", got)
	}
}

func TestHandleHandshakeSelfRegistration(t *testing.T) {
	mgr := testManager(t)
	_, err := mgr.HandleHandshake(models.FleetPeerPayload{
		NodeID: "test-node", Name: "Self", Address: "http://self.test", PeerSecret: "s",
	})
	if err == nil {
		t.Fatal("expected self-registration error")
	}
}

func TestHandleHeartbeatUnknownPeer(t *testing.T) {
	mgr := testManager(t)
	if err := mgr.HandleHeartbeat("missing", models.FleetPeerPayload{}); err == nil {
		t.Fatal("expected unknown peer error")
	}
}

func TestCheckPeersEmpty(t *testing.T) {
	mgr := testManager(t)
	mgr.checkPeers()
}

func TestStartStop(t *testing.T) {
	mgr := testManager(t)
	mgr.Start()
	time.Sleep(50 * time.Millisecond)
	mgr.Stop()
	mgr.Stop()
}

func TestPostPeerErrors(t *testing.T) {
	mgr := testManager(t)
	local, _ := mgr.LocalSettings()
	payload := models.FleetPeerPayload{NodeID: local.NodeID, Name: local.Name}

	if err := mgr.postPeer("://bad", local.PeerSecret, payload); err == nil {
		t.Fatal("expected postPeer error")
	}
	if _, err := mgr.postPeerJSON("://bad", "secret", payload); err == nil {
		t.Fatal("expected postPeerJSON error")
	}

	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	t.Cleanup(badServer.Close)
	if err := mgr.postPeer(badServer.URL, local.PeerSecret, payload); err == nil {
		t.Fatal("expected remote error")
	}
}

func TestListPeersResponseNilSlice(t *testing.T) {
	mgr := testManager(t)
	resp, err := mgr.ListPeersResponse()
	if err != nil {
		t.Fatal(err)
	}
	if resp.Peers == nil {
		t.Fatal("expected non-nil peers slice")
	}
}

func TestDefaultNameUsesServerName(t *testing.T) {
	db, err := database.New(filepath.Join(t.TempDir(), "fleet.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	notif := notifier.New(db, &config.NotifyConfig{}, "http://localhost:4376", "test")
	mgr, err := New(db, notif, config.ServerConfig{ServerName: "My Server", DomainURL: "http://localhost:4376"}, config.FleetConfig{NodeID: "node-1"})
	if err != nil {
		t.Fatal(err)
	}
	settings, _ := mgr.LocalSettings()
	if settings.Name != "My Server" {
		t.Fatalf("name=%q", settings.Name)
	}
}

func TestSetPeerStatusOfflineTransition(t *testing.T) {
	mgr := testManager(t)
	old := time.Now().UTC().Add(-2 * time.Hour)
	peer := &models.Peer{
		NodeID: "off", Name: "Off", Address: "http://off.test", PeerSecret: "s",
		Status: models.PeerStatusOnline, LastSeenAt: &old,
	}
	if err := mgr.db.UpsertPeer(peer); err != nil {
		t.Fatal(err)
	}
	mgr.mu.Lock()
	mgr.setPeerStatusLocked(*peer, models.PeerStatusOffline, &old)
	mgr.mu.Unlock()
	got, _ := mgr.db.GetPeer(peer.ID)
	if got.Status != models.PeerStatusOffline {
		t.Fatalf("status=%s", got.Status)
	}
}

func TestPostPeerJSONSuccess(t *testing.T) {
	mgr := testManager(t)
	local, _ := mgr.LocalSettings()
	payload := models.FleetPeerPayload{NodeID: local.NodeID, Name: local.Name, Address: mgr.callbackAddress()}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"node_id":"remote","name":"Remote","address":"http://remote.test"}`))
	}))
	t.Cleanup(srv.Close)

	info, err := mgr.postPeerJSON(srv.URL+"/api/fleet/handshake", local.PeerSecret, payload)
	if err != nil {
		t.Fatal(err)
	}
	if info.NodeID != "remote" {
		t.Fatalf("info=%+v", info)
	}
}

func TestValidatePeerURL(t *testing.T) {
	if err := validatePeerURL("ftp://bad"); err == nil {
		t.Fatal("expected scheme error")
	}
	if err := validatePeerURL("http://good.test"); err != nil {
		t.Fatal(err)
	}
}

func TestAddPeerRejectsSelfNodeID(t *testing.T) {
	mgr := testManager(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"node_id":"test-node","name":"Self","address":"http://x.test"}`))
	}))
	t.Cleanup(srv.Close)
	local, _ := mgr.LocalSettings()
	_, err := mgr.AddPeer("Self", srv.URL, local.PeerSecret)
	if err == nil || !strings.Contains(err.Error(), "itself") {
		t.Fatalf("err=%v", err)
	}
}

func TestPeerBySecret(t *testing.T) {
	mgr := testManager(t)
	peer := &models.Peer{
		NodeID: "lookup", Name: "Lookup", Address: "http://lookup.test", PeerSecret: "lookup-secret",
		Status: models.PeerStatusOnline,
	}
	if err := mgr.db.UpsertPeer(peer); err != nil {
		t.Fatal(err)
	}
	got, err := mgr.peerBySecret("lookup-secret")
	if err != nil || got == nil || got.NodeID != "lookup" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestHandleHandshakeMissingFields(t *testing.T) {
	mgr := testManager(t)
	_, err := mgr.HandleHandshake(models.FleetPeerPayload{})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestUpdateLocalRegenerateOnly(t *testing.T) {
	mgr := testManager(t)
	before, _ := mgr.PeerSecret()
	settings, err := mgr.UpdateLocal("", true)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := mgr.PeerSecret()
	if settings.PeerSecret == before || after == before {
		t.Fatal("expected regenerated secret")
	}
}

func TestIsBlockedPeerHostNames(t *testing.T) {
	if !isBlockedPeerHost("metadata.google.internal") {
		t.Fatal("expected metadata host blocked")
	}
}

func TestAddPeerHandshakeFailure(t *testing.T) {
	mgr := testManager(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	_, err := mgr.AddPeer("Bad", srv.URL, "secret")
	if err == nil || !strings.Contains(err.Error(), "handshake failed") {
		t.Fatalf("err=%v", err)
	}
}

func TestPeerWithoutLastSeen(t *testing.T) {
	mgr := testManager(t)
	peer := &models.Peer{
		NodeID: "noseen", Name: "NoSeen", Address: "http://noseen.test", PeerSecret: "s",
		Status: models.PeerStatusUnknown,
	}
	if err := mgr.db.UpsertPeer(peer); err != nil {
		t.Fatal(err)
	}
	got, _ := mgr.db.GetPeer(peer.ID)
	if got.LastSeenAt != nil {
		t.Fatal("expected nil last seen")
	}
}

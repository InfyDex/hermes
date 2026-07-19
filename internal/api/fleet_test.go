package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"

	"github.com/hermes-scheduler/hermes/internal/api"
	"github.com/hermes-scheduler/hermes/internal/auth"
	"github.com/hermes-scheduler/hermes/internal/config"
	"github.com/hermes-scheduler/hermes/internal/fleet"
	"github.com/hermes-scheduler/hermes/internal/models"
	"github.com/hermes-scheduler/hermes/internal/notifier"
	"github.com/hermes-scheduler/hermes/internal/testutil"
)

func newFleetAPI(t *testing.T) (*mux.Router, *fleet.Manager, string) {
	t.Helper()
	root, mgr, secret := newFleetAPIRoot(t)
	return root, mgr, secret
}

func newFleetAPIRoot(t *testing.T) (*mux.Router, *fleet.Manager, string) {
	t.Helper()
	db := testutil.TestDB(t)
	domainURL := "http://fleet-api.test"
	notif := notifier.New(db, &config.NotifyConfig{}, domainURL, "fleet-api")
	mgr, err := fleet.New(db, notif, config.ServerConfig{DomainURL: domainURL}, config.FleetConfig{NodeID: "fleet-api"})
	if err != nil {
		t.Fatalf("fleet.New: %v", err)
	}

	apiHandler := api.New(db, nil, nil, mgr)
	root := mux.NewRouter()
	router := root.PathPrefix("/api").Subrouter()
	apiHandler.RegisterFleetPublicRoutes(router)

	secret, _ := mgr.PeerSecret()
	router.Handle("/fleet/handshake", auth.PeerAuthMiddleware(func() string { return secret })(http.HandlerFunc(apiHandler.FleetHandshake))).Methods("POST")
	router.Handle("/fleet/heartbeat", auth.PeerHeartbeatMiddleware(mgr.AuthenticatePeerSecret)(http.HandlerFunc(apiHandler.FleetHeartbeat))).Methods("POST")

	admin := router.NewRoute().Subrouter()
	admin.Use(auth.BasicAuthMiddleware(testutil.TestAuthConfig()))
	apiHandler.RegisterFleetRoutes(admin)

	return root, mgr, secret
}

func TestFleetHealth(t *testing.T) {
	router, _, _ := newFleetAPI(t)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "ok" || resp["node_id"] != "fleet-api" {
		t.Fatalf("resp=%v", resp)
	}
}

func TestFleetLocalEndpoints(t *testing.T) {
	router, _, _ := newFleetAPI(t)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, authRequest(http.MethodGet, "/api/fleet/local", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("get local status=%d", rr.Code)
	}

	body, _ := json.Marshal(map[string]interface{}{
		"name":              "Renamed",
		"regenerate_secret": true,
	})
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, authRequest(http.MethodPut, "/api/fleet/local", body))
	if rr.Code != http.StatusOK {
		t.Fatalf("update local status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestFleetPeersListEmpty(t *testing.T) {
	router, _, _ := newFleetAPI(t)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, authRequest(http.MethodGet, "/api/fleet/peers", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var resp models.FleetPeersResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Peers) != 0 {
		t.Fatalf("peers=%d", len(resp.Peers))
	}
}

func TestFleetHandshakeAndHeartbeatAPI(t *testing.T) {
	router, mgr, secret := newFleetAPI(t)

	payload, _ := json.Marshal(models.FleetPeerPayload{
		NodeID:     "remote-node",
		Name:       "Remote",
		Address:    "http://remote.test:4376",
		PeerSecret: "remote-secret",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/fleet/handshake", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+secret)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("handshake status=%d body=%s", rr.Code, rr.Body.String())
	}

	hb, _ := json.Marshal(models.FleetPeerPayload{NodeID: "remote-node", Name: "Remote"})
	req = httptest.NewRequest(http.MethodPost, "/api/fleet/heartbeat", bytes.NewReader(hb))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer remote-secret")
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("heartbeat status=%d body=%s", rr.Code, rr.Body.String())
	}

	resp, err := mgr.ListPeersResponse()
	if err != nil || len(resp.Peers) != 1 {
		t.Fatalf("peers=%d err=%v", len(resp.Peers), err)
	}
}

func TestFleetDeletePeer(t *testing.T) {
	router, mgr, secret := newFleetAPI(t)
	_, err := mgr.HandleHandshake(models.FleetPeerPayload{
		NodeID: "del-node", Name: "Del", Address: "http://del.test:4376", PeerSecret: "s",
	})
	if err != nil {
		t.Fatal(err)
	}
	peers, _ := mgr.ListPeersResponse()
	if len(peers.Peers) != 1 {
		t.Fatal("expected peer")
	}

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, authRequest(http.MethodDelete, fmt.Sprintf("/api/fleet/peers/%d", peers.Peers[0].ID), nil))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d", rr.Code)
	}
	_ = secret
}

func TestFleetAddPeerValidation(t *testing.T) {
	router, _, _ := newFleetAPI(t)
	body, _ := json.Marshal(map[string]string{"name": "", "address": "", "peer_secret": ""})
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, authRequest(http.MethodPost, "/api/fleet/peers", body))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestFleetDeletePeerNotFound(t *testing.T) {
	router, _, _ := newFleetAPI(t)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, authRequest(http.MethodDelete, "/api/fleet/peers/9999", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestFleetHeartbeatInvalidToken(t *testing.T) {
	router, _, _ := newFleetAPI(t)
	body, _ := json.Marshal(models.FleetPeerPayload{NodeID: "x"})
	req := httptest.NewRequest(http.MethodPost, "/api/fleet/heartbeat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer wrong")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestFleetHeartbeatBadPayload(t *testing.T) {
	router, mgr, _ := newFleetAPI(t)
	_, _ = mgr.HandleHandshake(models.FleetPeerPayload{
		NodeID: "hb-node", Name: "HB", Address: "http://hb.test:4376", PeerSecret: "remote-secret",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/fleet/heartbeat", bytes.NewReader([]byte(`{"node_id":"other"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer remote-secret")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestFleetUpdateLocalInvalidJSON(t *testing.T) {
	router, _, _ := newFleetAPI(t)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, authRequest(http.MethodPut, "/api/fleet/local", []byte(`{`)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestFleetHandshakeInvalidBody(t *testing.T) {
	router, _, secret := newFleetAPI(t)
	req := httptest.NewRequest(http.MethodPost, "/api/fleet/handshake", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+secret)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestFleetDeletePeerInvalidID(t *testing.T) {
	router, _, _ := newFleetAPI(t)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, authRequest(http.MethodDelete, "/api/fleet/peers/bad", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestFleetTwoNodeAddPeerAPI(t *testing.T) {
	remoteRoot, _, remoteSecret := newFleetAPIRoot(t)
	remoteServer := httptest.NewServer(remoteRoot)
	t.Cleanup(remoteServer.Close)

	db := testutil.TestDB(t)
	domainURL := "http://local.test"
	notif := notifier.New(db, &config.NotifyConfig{}, domainURL, "local")
	localMgr, err := fleet.New(db, notif, config.ServerConfig{DomainURL: domainURL}, config.FleetConfig{NodeID: "local"})
	if err != nil {
		t.Fatal(err)
	}
	apiHandler := api.New(db, nil, nil, localMgr)
	router := mux.NewRouter()
	apiRouter := router.PathPrefix("/api").Subrouter()
	admin := apiRouter.NewRoute().Subrouter()
	admin.Use(auth.BasicAuthMiddleware(testutil.TestAuthConfig()))
	apiHandler.RegisterFleetRoutes(admin)

	body, _ := json.Marshal(map[string]string{
		"name":        "Remote",
		"address":     remoteServer.URL,
		"peer_secret": remoteSecret,
	})
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, authRequest(http.MethodPost, "/api/fleet/peers", body))
	if rr.Code != http.StatusCreated {
		t.Fatalf("add peer status=%d body=%s", rr.Code, rr.Body.String())
	}
}

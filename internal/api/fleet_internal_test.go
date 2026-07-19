package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"

	"github.com/hermes-scheduler/hermes/internal/config"
	"github.com/hermes-scheduler/hermes/internal/database"
	"github.com/hermes-scheduler/hermes/internal/fleet"
	"github.com/hermes-scheduler/hermes/internal/models"
	"github.com/hermes-scheduler/hermes/internal/notifier"
)

func testFleetAPI(t *testing.T) *API {
	t.Helper()
	db, err := database.New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	notif := notifier.New(db, &config.NotifyConfig{}, "http://localhost:4376", "test")
	mgr, err := fleet.New(db, notif, config.ServerConfig{DomainURL: "http://localhost:4376"}, config.FleetConfig{NodeID: "api-test"})
	if err != nil {
		t.Fatal(err)
	}
	return New(db, nil, nil, mgr)
}

func TestDecodeFleetBody(t *testing.T) {
	a := testFleetAPI(t)
	_ = a

	w := httptest.NewRecorder()
	body := `{"node_id":"n","name":"N","address":"http://n.test","peer_secret":"s"}`
	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(body)))
	var payload models.FleetPeerPayload
	if err := decodeFleetBody(w, r, &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.NodeID != "n" {
		t.Fatalf("payload=%+v", payload)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(body+`{"extra":true}`)))
	if err := decodeFleetBody(w, r, &payload); err == nil {
		t.Fatal("expected trailing JSON error")
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`not-json`)))
	if err := decodeFleetBody(w, r, &payload); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}

func TestFleetHandlerWrappers(t *testing.T) {
	a := testFleetAPI(t)
	router := mux.NewRouter()
	a.RegisterFleetPublicRoutes(router)
	a.RegisterFleetRoutes(router)

	w := httptest.NewRecorder()
	a.FleetHandshake(w, httptest.NewRequest(http.MethodPost, "/fleet/handshake", bytes.NewReader([]byte(`{}`))))
	a.FleetHeartbeat(w, httptest.NewRequest(http.MethodPost, "/fleet/heartbeat", nil))
}

package web_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/hermes-scheduler/hermes/internal/models"
)

func TestFleetPage(t *testing.T) {
	router, store, _ := newTestWeb(t)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, authenticatedRequest(t, store, http.MethodGet, "/fleet", ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Fleet Settings") {
		t.Fatal("expected fleet page")
	}
}

func TestFleetStatusJSON(t *testing.T) {
	router, store, _ := newTestWeb(t)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, authenticatedRequest(t, store, http.MethodGet, "/fleet/status", ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var resp models.FleetPeersResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Local.NodeID == "" {
		t.Fatalf("resp=%+v", resp)
	}
}

func TestFleetUpdateLocal(t *testing.T) {
	router, store, _ := newTestWeb(t)

	getReq := authenticatedRequest(t, store, http.MethodGet, "/fleet", "")
	pageRR := httptest.NewRecorder()
	router.ServeHTTP(pageRR, getReq)
	if pageRR.Code != http.StatusOK {
		t.Fatalf("fleet page status=%d", pageRR.Code)
	}
	csrf := extractCSRFFromBody(t, pageRR.Body.String())

	form := url.Values{}
	form.Set("name", "Updated Node")
	form.Set("csrf_token", csrf)
	req := httptest.NewRequest(http.MethodPost, "/fleet", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range getReq.Cookies() {
		req.AddCookie(c)
	}
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestFleetAddPeerInvalidCSRF(t *testing.T) {
	router, store, _ := newTestWeb(t)
	form := url.Values{}
	form.Set("name", "bad")
	form.Set("address", "http://x.test")
	form.Set("peer_secret", "secret")
	form.Set("csrf_token", "wrong")
	req := authenticatedRequest(t, store, http.MethodPost, "/fleet/peers", form.Encode())
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestDashboardShowsFleetPanel(t *testing.T) {
	router, store, _ := newTestWeb(t)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, authenticatedRequest(t, store, http.MethodGet, "/", ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "fleet-panel") {
		t.Fatal("expected fleet panel on dashboard")
	}
}

func TestFleetDeletePeer(t *testing.T) {
	router, store, deps := newTestWeb(t)
	now := time.Now().UTC()
	peer := &models.Peer{
		NodeID: "del", Name: "Del", Address: "http://del.test", PeerSecret: "s",
		Status: models.PeerStatusOnline, LastSeenAt: &now,
	}
	if err := deps.DB.UpsertPeer(peer); err != nil {
		t.Fatal(err)
	}

	getReq := authenticatedRequest(t, store, http.MethodGet, "/fleet", "")
	pageRR := httptest.NewRecorder()
	router.ServeHTTP(pageRR, getReq)
	csrf := extractCSRFFromBody(t, pageRR.Body.String())

	form := url.Values{}
	form.Set("csrf_token", csrf)
	req := httptest.NewRequest(http.MethodPost, "/fleet/peers/1/delete", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range getReq.Cookies() {
		req.AddCookie(c)
	}
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestFleetUpdateLocalRegenerateSecret(t *testing.T) {
	router, store, _ := newTestWeb(t)

	getReq := authenticatedRequest(t, store, http.MethodGet, "/fleet", "")
	pageRR := httptest.NewRecorder()
	router.ServeHTTP(pageRR, getReq)
	csrf := extractCSRFFromBody(t, pageRR.Body.String())

	form := url.Values{}
	form.Set("name", "Regen Node")
	form.Set("regenerate_secret", "on")
	form.Set("csrf_token", csrf)
	req := httptest.NewRequest(http.MethodPost, "/fleet", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range getReq.Cookies() {
		req.AddCookie(c)
	}
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestFleetPageWithMessages(t *testing.T) {
	router, store, _ := newTestWeb(t)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, authenticatedRequest(t, store, http.MethodGet, "/fleet?error=oops&notice=hi", ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "oops") || !strings.Contains(rr.Body.String(), "hi") {
		t.Fatal("expected fleet messages rendered")
	}
}

func TestFleetDeletePeerInvalidID(t *testing.T) {
	router, store, _ := newTestWeb(t)
	getReq := authenticatedRequest(t, store, http.MethodGet, "/fleet", "")
	pageRR := httptest.NewRecorder()
	router.ServeHTTP(pageRR, getReq)
	csrf := extractCSRFFromBody(t, pageRR.Body.String())

	form := url.Values{}
	form.Set("csrf_token", csrf)
	req := httptest.NewRequest(http.MethodPost, "/fleet/peers/bad/delete", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range getReq.Cookies() {
		req.AddCookie(c)
	}
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

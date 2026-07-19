package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	"github.com/gorilla/mux"

	"github.com/hermes-scheduler/hermes/internal/auth"
)

func (w *Web) fleetPage(wr http.ResponseWriter, r *http.Request) {
	settings, err := w.fleet.LocalSettings()
	if err != nil || settings == nil {
		http.Error(wr, "fleet not initialized", http.StatusInternalServerError)
		return
	}
	peers, _ := w.db.ListPeers()
	w.renderPage(wr, r, "fleet", map[string]interface{}{
		"Title":       "Fleet Settings",
		"Node":        settings,
		"LocalAddr":   w.fleet.LocalAddress(),
		"Peers":       peers,
		"DomainURL":   w.serverCfg.DomainURL,
		"FleetError":  r.URL.Query().Get("error"),
		"FleetNotice": r.URL.Query().Get("notice"),
	})
}

func (w *Web) fleetUpdateLocal(wr http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(wr, "bad request", http.StatusBadRequest)
		return
	}
	session, ok := auth.SessionFromContext(r.Context())
	if !ok || !auth.ValidateCSRF(session, r.FormValue("csrf_token")) {
		http.Error(wr, "invalid csrf token", http.StatusForbidden)
		return
	}
	_, err := w.fleet.UpdateLocal(r.FormValue("name"), r.FormValue("regenerate_secret") == "on")
	if err != nil {
		http.Redirect(wr, r, "/fleet?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(wr, r, "/fleet?notice=Node+settings+updated", http.StatusSeeOther)
}

func (w *Web) fleetAddPeer(wr http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(wr, "bad request", http.StatusBadRequest)
		return
	}
	session, ok := auth.SessionFromContext(r.Context())
	if !ok || !auth.ValidateCSRF(session, r.FormValue("csrf_token")) {
		http.Error(wr, "invalid csrf token", http.StatusForbidden)
		return
	}
	_, err := w.fleet.AddPeer(r.FormValue("name"), r.FormValue("address"), r.FormValue("peer_secret"))
	if err != nil {
		http.Redirect(wr, r, "/?fleet_error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(wr, r, "/?fleet_notice=Peer+connected", http.StatusSeeOther)
}

func (w *Web) fleetDeletePeer(wr http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(wr, "bad request", http.StatusBadRequest)
		return
	}
	session, ok := auth.SessionFromContext(r.Context())
	if !ok || !auth.ValidateCSRF(session, r.FormValue("csrf_token")) {
		http.Error(wr, "invalid csrf token", http.StatusForbidden)
		return
	}
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		http.Error(wr, "invalid id", http.StatusBadRequest)
		return
	}
	if err := w.fleet.RemovePeer(id); err != nil {
		http.Redirect(wr, r, "/fleet?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(wr, r, "/fleet?notice=Peer+removed", http.StatusSeeOther)
}

func (w *Web) fleetStatusJSON(wr http.ResponseWriter, r *http.Request) {
	resp, err := w.fleet.ListPeersResponse()
	if err != nil {
		http.Error(wr, err.Error(), http.StatusInternalServerError)
		return
	}
	wr.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(wr).Encode(resp)
}

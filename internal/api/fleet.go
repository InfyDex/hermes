package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/hermes-scheduler/hermes/internal/models"
)

const maxFleetBodyBytes = 16 << 10 // 16 KiB

func (a *API) RegisterFleetPublicRoutes(r *mux.Router) {
	r.HandleFunc("/health", a.health).Methods("GET")
}

func (a *API) FleetHandshake(w http.ResponseWriter, r *http.Request) {
	a.fleetHandshake(w, r)
}

func (a *API) FleetHeartbeat(w http.ResponseWriter, r *http.Request) {
	a.fleetHeartbeat(w, r)
}

func (a *API) RegisterFleetRoutes(r *mux.Router) {
	r.HandleFunc("/fleet/peers", a.listFleetPeers).Methods("GET")
	r.HandleFunc("/fleet/peers", a.addFleetPeer).Methods("POST")
	r.HandleFunc("/fleet/peers/{id}", a.deleteFleetPeer).Methods("DELETE")
	r.HandleFunc("/fleet/local", a.getFleetLocal).Methods("GET")
	r.HandleFunc("/fleet/local", a.updateFleetLocal).Methods("PUT")
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	nodeID := ""
	if settings, err := a.fleet.LocalSettings(); err == nil && settings != nil {
		nodeID = settings.NodeID
	}
	jsonResponse(w, map[string]string{"status": "ok", "node_id": nodeID}, http.StatusOK)
}

func (a *API) fleetHandshake(w http.ResponseWriter, r *http.Request) {
	var payload models.FleetPeerPayload
	if err := decodeFleetBody(w, r, &payload); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	info, err := a.fleet.HandleHandshake(payload)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	jsonResponse(w, info, http.StatusOK)
}

func (a *API) fleetHeartbeat(w http.ResponseWriter, r *http.Request) {
	var payload models.FleetPeerPayload
	if err := decodeFleetBody(w, r, &payload); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.fleet.HandleHeartbeat(payload); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) listFleetPeers(w http.ResponseWriter, r *http.Request) {
	resp, err := a.fleet.ListPeersResponse()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, resp, http.StatusOK)
}

func (a *API) addFleetPeer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string `json:"name"`
		Address    string `json:"address"`
		PeerSecret string `json:"peer_secret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	peer, err := a.fleet.AddPeer(req.Name, req.Address, req.PeerSecret)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	peer.PeerSecret = ""
	jsonResponse(w, peer, http.StatusCreated)
}

func (a *API) deleteFleetPeer(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := a.fleet.RemovePeer(id); err != nil {
		jsonError(w, err.Error(), http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) getFleetLocal(w http.ResponseWriter, r *http.Request) {
	settings, err := a.fleet.LocalSettings()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if settings == nil {
		jsonError(w, "not initialized", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, map[string]interface{}{
		"node_id":     settings.NodeID,
		"name":        settings.Name,
		"peer_secret": settings.PeerSecret,
		"address":     a.fleet.LocalAddress(),
	}, http.StatusOK)
}

func (a *API) updateFleetLocal(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name             string `json:"name"`
		RegenerateSecret bool   `json:"regenerate_secret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	settings, err := a.fleet.UpdateLocal(req.Name, req.RegenerateSecret)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	jsonResponse(w, map[string]interface{}{
		"node_id":     settings.NodeID,
		"name":        settings.Name,
		"peer_secret": settings.PeerSecret,
		"address":     a.fleet.LocalAddress(),
	}, http.StatusOK)
}

func decodeFleetBody(w http.ResponseWriter, r *http.Request, v interface{}) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxFleetBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("invalid request body")
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("invalid request body")
	}
	return nil
}
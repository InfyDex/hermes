package fleet

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/hermes-scheduler/hermes/internal/config"
	"github.com/hermes-scheduler/hermes/internal/database"
	"github.com/hermes-scheduler/hermes/internal/models"
	"github.com/hermes-scheduler/hermes/internal/notifier"
	"github.com/hermes-scheduler/hermes/internal/auth"
)

const (
	heartbeatInterval = 30 * time.Second
	offlineThreshold  = 3
	httpTimeout       = 10 * time.Second
	notifyCooldown    = 60 * time.Second
)

type Manager struct {
	db       *database.DB
	notifier *notifier.Notifier
	server   config.ServerConfig
	fleet    config.FleetConfig
	client   *http.Client

	mu                sync.Mutex
	missCounts        map[int64]int
	lastOfflineNotify map[int64]time.Time
	lastOnlineNotify  map[int64]time.Time
	stopCh            chan struct{}
}

func New(db *database.DB, notif *notifier.Notifier, server config.ServerConfig, fleet config.FleetConfig) (*Manager, error) {
	m := &Manager{
		db:         db,
		notifier:   notif,
		server:     server,
		fleet:      fleet,
		client: &http.Client{
			Timeout: httpTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return fmt.Errorf("redirect not allowed")
			},
		},
		missCounts:        make(map[int64]int),
		lastOfflineNotify: make(map[int64]time.Time),
		lastOnlineNotify:  make(map[int64]time.Time),
		stopCh:     make(chan struct{}),
	}
	if err := m.ensureNodeSettings(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Manager) ensureNodeSettings() error {
	settings, err := m.db.GetNodeSettings()
	if err != nil {
		return err
	}
	if settings != nil {
		return nil
	}

	nodeID := m.defaultNodeID()
	name := m.defaultName(nodeID)
	secret, err := generateSecret()
	if err != nil {
		return err
	}
	return m.db.SaveNodeSettings(&models.NodeSettings{
		NodeID:     nodeID,
		Name:       name,
		PeerSecret: secret,
	})
}

func (m *Manager) defaultNodeID() string {
	if m.fleet.NodeID != "" {
		return m.fleet.NodeID
	}
	if m.server.ServerName != "" {
		return sanitizeID(m.server.ServerName)
	}
	host, err := os.Hostname()
	if err != nil || host == "" {
		return "hermes-node"
	}
	return sanitizeID(host)
}

func (m *Manager) defaultName(nodeID string) string {
	if m.server.ServerName != "" {
		return m.server.ServerName
	}
	return nodeID
}

func sanitizeID(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	return s
}

func (m *Manager) LocalSettings() (*models.NodeSettings, error) {
	return m.db.GetNodeSettings()
}

func (m *Manager) LocalAddress() string {
	if m.server.DomainURL != "" {
		return NormalizeAddress(m.server.DomainURL)
	}
	return ""
}

func (m *Manager) LocalNodeInfo() (models.FleetNodeInfo, error) {
	settings, err := m.db.GetNodeSettings()
	if err != nil {
		return models.FleetNodeInfo{}, err
	}
	if settings == nil {
		return models.FleetNodeInfo{}, fmt.Errorf("node settings not initialized")
	}
	return models.FleetNodeInfo{
		NodeID:  settings.NodeID,
		Name:    settings.Name,
		Address: m.LocalAddress(),
	}, nil
}

func (m *Manager) PeerSecret() (string, error) {
	settings, err := m.db.GetNodeSettings()
	if err != nil {
		return "", err
	}
	if settings == nil {
		return "", fmt.Errorf("node settings not initialized")
	}
	return settings.PeerSecret, nil
}

func (m *Manager) ListPeersResponse() (*models.FleetPeersResponse, error) {
	local, err := m.LocalNodeInfo()
	if err != nil {
		return nil, err
	}
	peers, err := m.db.ListPeers()
	if err != nil {
		return nil, err
	}
	if peers == nil {
		peers = []models.Peer{}
	}
	for i := range peers {
		peers[i].PeerSecret = ""
	}
	return &models.FleetPeersResponse{
		Local:  local,
		Peers:  peers,
		Domain: m.server.DomainURL,
	}, nil
}

func (m *Manager) AddPeer(name, address, peerSecret string) (*models.Peer, error) {
	name = strings.TrimSpace(name)
	address = NormalizeAddress(address)
	peerSecret = strings.TrimSpace(peerSecret)
	if name == "" || address == "" || peerSecret == "" {
		return nil, fmt.Errorf("display name, peer URL, and remote secret are required")
	}
	if err := validatePeerURL(address); err != nil {
		return nil, err
	}

	local, err := m.LocalSettings()
	if err != nil {
		return nil, err
	}
	if local == nil {
		return nil, fmt.Errorf("node settings not initialized")
	}

	callback := m.callbackAddress()
	if callback == "" {
		return nil, fmt.Errorf("set HERMES_DOMAIN_URL to this node's public URL before adding peers")
	}

	payload := models.FleetPeerPayload{
		NodeID:     local.NodeID,
		Name:       local.Name,
		Address:    callback,
		PeerSecret: local.PeerSecret,
	}
	respInfo, err := m.postPeerJSON(address+"/api/fleet/handshake", peerSecret, payload)
	if err != nil {
		return nil, fmt.Errorf("handshake failed: %w", err)
	}
	if respInfo.NodeID == local.NodeID {
		return nil, fmt.Errorf("cannot add this node as a peer of itself")
	}

	// Outbound callbacks use the admin-entered URL, not the remote handshake response.
	peerAddress := address

	peer := &models.Peer{
		NodeID:     respInfo.NodeID,
		Name:       respInfo.Name,
		Address:    peerAddress,
		PeerSecret: peerSecret,
		Status:     models.PeerStatusOnline,
	}
	now := time.Now().UTC()
	peer.LastSeenAt = &now
	if err := m.db.UpsertPeer(peer); err != nil {
		return nil, fmt.Errorf("handshake ok but failed to save peer locally (retry add peer): %w", err)
	}
	m.mu.Lock()
	m.missCounts[peer.ID] = 0
	m.mu.Unlock()
	return peer, nil
}

func (m *Manager) callbackAddress() string {
	if m.server.DomainURL != "" {
		return NormalizeAddress(m.server.DomainURL)
	}
	return ""
}

func (m *Manager) RemovePeer(id int64) error {
	m.mu.Lock()
	delete(m.missCounts, id)
	delete(m.lastOfflineNotify, id)
	delete(m.lastOnlineNotify, id)
	m.mu.Unlock()
	return m.db.DeletePeer(id)
}

func (m *Manager) HandleHandshake(payload models.FleetPeerPayload) (models.FleetNodeInfo, error) {
	payload.NodeID = strings.TrimSpace(payload.NodeID)
	payload.Name = strings.TrimSpace(payload.Name)
	payload.Address = NormalizeAddress(payload.Address)
	payload.PeerSecret = strings.TrimSpace(payload.PeerSecret)

	if payload.NodeID == "" || payload.Name == "" || payload.Address == "" || payload.PeerSecret == "" {
		return models.FleetNodeInfo{}, fmt.Errorf("node_id, name, address, and peer_secret are required")
	}
	if err := validatePeerCallbackURL(payload.Address); err != nil {
		return models.FleetNodeInfo{}, err
	}

	local, err := m.LocalSettings()
	if err != nil {
		return models.FleetNodeInfo{}, err
	}
	if local == nil {
		return models.FleetNodeInfo{}, fmt.Errorf("node settings not initialized")
	}
	if payload.NodeID == local.NodeID {
		return models.FleetNodeInfo{}, fmt.Errorf("cannot register this node as its own peer")
	}

	now := time.Now().UTC()
	peer := &models.Peer{
		NodeID:     payload.NodeID,
		Name:       payload.Name,
		Address:    payload.Address,
		PeerSecret: payload.PeerSecret,
		Status:     models.PeerStatusOnline,
		LastSeenAt: &now,
	}
	if err := m.db.UpsertPeer(peer); err != nil {
		return models.FleetNodeInfo{}, err
	}
	m.mu.Lock()
	m.missCounts[peer.ID] = 0
	m.mu.Unlock()

	return m.LocalNodeInfo()
}

func (m *Manager) HandleHeartbeat(token string, payload models.FleetPeerPayload) error {
	peer, err := m.peerBySecret(token)
	if err != nil {
		return err
	}
	if peer == nil {
		return fmt.Errorf("unknown peer")
	}

	payload.NodeID = strings.TrimSpace(payload.NodeID)
	if payload.NodeID != "" && payload.NodeID != peer.NodeID {
		return fmt.Errorf("node_id mismatch")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	current, err := m.db.GetPeer(peer.ID)
	if err != nil || current == nil {
		return fmt.Errorf("unknown peer")
	}
	prevStatus := current.Status

	now := time.Now().UTC()
	if payload.Name != "" {
		current.Name = strings.TrimSpace(payload.Name)
	}
	if addr := NormalizeAddress(payload.Address); addr != "" {
		if err := validatePeerCallbackURL(addr); err == nil {
			current.Address = addr
		}
	}
	current.Status = models.PeerStatusOnline
	current.LastSeenAt = &now
	if err := m.db.UpsertPeer(current); err != nil {
		return err
	}
	m.missCounts[current.ID] = 0
	m.maybeNotifyTransitionLocked(current.ID, current.Name, prevStatus, models.PeerStatusOnline)
	return nil
}

func (m *Manager) AuthenticatePeerSecret(token string) bool {
	peer, err := m.peerBySecret(token)
	return err == nil && peer != nil
}

func (m *Manager) peerBySecret(secret string) (*models.Peer, error) {
	peers, err := m.db.ListPeers()
	if err != nil {
		return nil, err
	}
	var matched *models.Peer
	for i := range peers {
		if auth.SafeSecretEqual(secret, peers[i].PeerSecret) {
			p := peers[i]
			matched = &p
		}
	}
	return matched, nil
}

func (m *Manager) UpdateLocal(name string, regenerateSecret bool) (*models.NodeSettings, error) {
	settings, err := m.db.GetNodeSettings()
	if err != nil {
		return nil, err
	}
	if settings == nil {
		return nil, fmt.Errorf("node settings not initialized")
	}
	if name = strings.TrimSpace(name); name != "" {
		settings.Name = name
	}
	if regenerateSecret {
		secret, err := generateSecret()
		if err != nil {
			return nil, err
		}
		settings.PeerSecret = secret
	}
	if err := m.db.SaveNodeSettings(settings); err != nil {
		return nil, err
	}
	return settings, nil
}

func (m *Manager) Start() {
	go m.loop()
}

func (m *Manager) Stop() {
	select {
	case <-m.stopCh:
	default:
		close(m.stopCh)
	}
}

func (m *Manager) loop() {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	m.checkPeers()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.checkPeers()
		}
	}
}

func (m *Manager) checkPeers() {
	peers, err := m.db.ListPeers()
	if err != nil {
		log.Printf("fleet: list peers: %v", err)
		return
	}
	if len(peers) == 0 {
		return
	}

	local, err := m.LocalSettings()
	if err != nil || local == nil {
		log.Printf("fleet: local settings: %v", err)
		return
	}

	payload := models.FleetPeerPayload{
		NodeID:  local.NodeID,
		Name:    local.Name,
		Address: m.callbackAddress(),
	}

	var wg sync.WaitGroup
	for _, peer := range peers {
		wg.Add(1)
		go func(p models.Peer) {
			defer wg.Done()
			m.pingPeer(p, local, payload)
		}(peer)
	}
	wg.Wait()
}

func (m *Manager) pingPeer(peer models.Peer, local *models.NodeSettings, payload models.FleetPeerPayload) {
	err := m.postPeer(addressJoin(peer.Address, "/api/fleet/heartbeat"), local.PeerSecret, payload)
	now := time.Now().UTC()

	m.mu.Lock()
	defer m.mu.Unlock()

	if err != nil {
		current, loadErr := m.db.GetPeer(peer.ID)
		if loadErr == nil && current != nil && current.LastSeenAt != nil {
			grace := heartbeatInterval * time.Duration(offlineThreshold)
			if time.Since(*current.LastSeenAt) < grace {
				return
			}
		}
		m.missCounts[peer.ID]++
		if m.missCounts[peer.ID] >= offlineThreshold {
			var lastSeen *time.Time
			if current != nil {
				lastSeen = current.LastSeenAt
			}
			m.setPeerStatusLocked(peer, models.PeerStatusOffline, lastSeen)
		}
		return
	}

	m.missCounts[peer.ID] = 0
	m.setPeerStatusLocked(peer, models.PeerStatusOnline, &now)
}

func (m *Manager) setPeerStatusLocked(peer models.Peer, status models.PeerStatus, lastSeen *time.Time) {
	current, err := m.db.GetPeer(peer.ID)
	if err != nil || current == nil {
		log.Printf("fleet: load peer %d status: %v", peer.ID, err)
		return
	}
	prev := current.Status
	if prev == status {
		if status == models.PeerStatusOnline && lastSeen != nil {
			if err := m.db.UpdatePeerStatus(peer.ID, status, lastSeen); err != nil {
				log.Printf("fleet: refresh peer %d last_seen: %v", peer.ID, err)
			}
		}
		return
	}
	if status == models.PeerStatusOffline && current.LastSeenAt != nil {
		grace := heartbeatInterval * time.Duration(offlineThreshold)
		if time.Since(*current.LastSeenAt) < grace {
			return
		}
	}
	if err := m.db.UpdatePeerStatus(peer.ID, status, lastSeen); err != nil {
		log.Printf("fleet: update peer %d status: %v", peer.ID, err)
		return
	}
	m.maybeNotifyTransitionLocked(peer.ID, current.Name, prev, status)
}

func (m *Manager) maybeNotifyTransitionLocked(peerID int64, name string, prev, next models.PeerStatus) {
	if prev == next {
		return
	}
	var title, body string
	switch next {
	case models.PeerStatusOffline:
		title = "Peer Offline: " + name
		body = name + " is unreachable."
	case models.PeerStatusOnline:
		if prev != models.PeerStatusOffline && prev != models.PeerStatusUnknown {
			return
		}
		title = "Peer Online: " + name
		body = name + " is back online."
	default:
		return
	}
	var last map[int64]time.Time
	switch next {
	case models.PeerStatusOffline:
		last = m.lastOfflineNotify
	case models.PeerStatusOnline:
		last = m.lastOnlineNotify
	}
	if t, ok := last[peerID]; ok && time.Since(t) < notifyCooldown {
		return
	}
	last[peerID] = time.Now()
	m.notifier.SystemNotify(title, body)
}

func (m *Manager) postPeer(target, secret string, payload models.FleetPeerPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+secret)

	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("remote returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return nil
}

func (m *Manager) postPeerJSON(target, secret string, payload models.FleetPeerPayload) (models.FleetNodeInfo, error) {
	var info models.FleetNodeInfo
	body, err := json.Marshal(payload)
	if err != nil {
		return info, err
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return info, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+secret)

	resp, err := m.client.Do(req)
	if err != nil {
		return info, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return info, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return info, fmt.Errorf("remote returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	if len(respBody) > 0 {
		if err := json.Unmarshal(respBody, &info); err != nil {
			return info, err
		}
	}
	return info, nil
}

func NormalizeAddress(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return strings.TrimRight(raw, "/")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	if u.Path == "/" {
		u.Path = ""
	}
	return strings.TrimRight(u.String(), "/")
}

func addressJoin(base, path string) string {
	return strings.TrimRight(base, "/") + path
}

func validatePeerURL(address string) error {
	u, err := url.Parse(address)
	if err != nil || u.Host == "" {
		return fmt.Errorf("invalid peer URL")
	}
	switch u.Scheme {
	case "http", "https":
		return nil
	default:
		return fmt.Errorf("peer URL must use http or https")
	}
}

func validatePeerCallbackURL(address string) error {
	if err := validatePeerURL(address); err != nil {
		return err
	}
	if isBlockedPeerHost(peerHost(address)) {
		return fmt.Errorf("peer callback URL is not allowed")
	}
	return nil
}

func peerHost(address string) string {
	u, err := url.Parse(address)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

func isBlockedPeerHost(host string) bool {
	switch host {
	case "localhost", "169.254.169.254", "metadata.google.internal", "metadata":
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsLinkLocalUnicast()
}

func generateSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

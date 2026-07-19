package database_test

import (
	"testing"
	"time"

	"github.com/hermes-scheduler/hermes/internal/models"
	"github.com/hermes-scheduler/hermes/internal/testutil"
)

func TestNodeSettingsCRUD(t *testing.T) {
	db := testutil.TestDB(t)

	if _, err := db.GetNodeSettings(); err != nil {
		t.Fatalf("GetNodeSettings: %v", err)
	}

	settings := &models.NodeSettings{
		NodeID:     "node-1",
		Name:       "Node One",
		PeerSecret: "secret-a",
	}
	if err := db.SaveNodeSettings(settings); err != nil {
		t.Fatalf("SaveNodeSettings: %v", err)
	}

	got, err := db.GetNodeSettings()
	if err != nil || got == nil || got.NodeID != "node-1" {
		t.Fatalf("GetNodeSettings = %+v, err=%v", got, err)
	}

	settings.Name = "Updated"
	if err := db.SaveNodeSettings(settings); err != nil {
		t.Fatalf("SaveNodeSettings update: %v", err)
	}
	got, _ = db.GetNodeSettings()
	if got.Name != "Updated" {
		t.Fatalf("name = %q", got.Name)
	}
}

func TestPeerCRUD(t *testing.T) {
	db := testutil.TestDB(t)
	now := time.Now().UTC()

	peer := &models.Peer{
		NodeID:     "remote-1",
		Name:       "Remote",
		Address:    "http://remote.test:4376",
		PeerSecret: "peer-secret",
		Status:     models.PeerStatusOnline,
		LastSeenAt: &now,
	}
	if err := db.UpsertPeer(peer); err != nil {
		t.Fatalf("UpsertPeer: %v", err)
	}
	if peer.ID == 0 {
		t.Fatal("expected peer ID")
	}

	got, err := db.GetPeer(peer.ID)
	if err != nil || got == nil || got.NodeID != "remote-1" {
		t.Fatalf("GetPeer = %+v, err=%v", got, err)
	}

	byNode, err := db.GetPeerByNodeID("remote-1")
	if err != nil || byNode == nil || byNode.ID != peer.ID {
		t.Fatalf("GetPeerByNodeID = %+v, err=%v", byNode, err)
	}

	byAddr, err := db.GetPeerByAddress("http://remote.test:4376")
	if err != nil || byAddr == nil || byAddr.ID != peer.ID {
		t.Fatalf("GetPeerByAddress = %+v, err=%v", byAddr, err)
	}

	peers, err := db.ListPeers()
	if err != nil || len(peers) != 1 {
		t.Fatalf("ListPeers = %d, err=%v", len(peers), err)
	}

	if err := db.UpdatePeerStatus(peer.ID, models.PeerStatusOffline, &now); err != nil {
		t.Fatalf("UpdatePeerStatus: %v", err)
	}
	got, _ = db.GetPeer(peer.ID)
	if got.Status != models.PeerStatusOffline {
		t.Fatalf("status = %s", got.Status)
	}

	if err := db.DeletePeer(peer.ID); err != nil {
		t.Fatalf("DeletePeer: %v", err)
	}
	peers, _ = db.ListPeers()
	if len(peers) != 0 {
		t.Fatalf("peers after delete = %d", len(peers))
	}
}

func TestUpsertPeerByNodeIDUpdatesAddress(t *testing.T) {
	db := testutil.TestDB(t)
	now := time.Now().UTC()

	first := &models.Peer{
		NodeID:     "same-node",
		Name:       "Peer",
		Address:    "http://old.test:4376",
		PeerSecret: "secret-1",
		Status:     models.PeerStatusOnline,
		LastSeenAt: &now,
	}
	if err := db.UpsertPeer(first); err != nil {
		t.Fatalf("UpsertPeer first: %v", err)
	}

	second := &models.Peer{
		NodeID:     "same-node",
		Name:       "Peer Renamed",
		Address:    "http://new.test:4376",
		PeerSecret: "secret-2",
		Status:     models.PeerStatusOnline,
		LastSeenAt: &now,
	}
	if err := db.UpsertPeer(second); err != nil {
		t.Fatalf("UpsertPeer second: %v", err)
	}

	peers, err := db.ListPeers()
	if err != nil || len(peers) != 1 {
		t.Fatalf("ListPeers = %+v, err=%v", peers, err)
	}
	if peers[0].Address != "http://new.test:4376" || peers[0].Name != "Peer Renamed" {
		t.Fatalf("peer = %+v", peers[0])
	}
}

func TestUpsertPeerByAddress(t *testing.T) {
	db := testutil.TestDB(t)

	peer := &models.Peer{
		NodeID:     "node-x",
		Name:       "X",
		Address:    "http://x.test:4376",
		PeerSecret: "s1",
		Status:     models.PeerStatusUnknown,
	}
	if err := db.UpsertPeer(peer); err != nil {
		t.Fatal(err)
	}

	updated := &models.Peer{
		NodeID:     "node-x-updated",
		Name:       "X2",
		Address:    "http://x.test:4376",
		PeerSecret: "s2",
		Status:     models.PeerStatusOnline,
	}
	if err := db.UpsertPeer(updated); err != nil {
		t.Fatal(err)
	}
	if updated.ID != peer.ID {
		t.Fatalf("id = %d want %d", updated.ID, peer.ID)
	}
}

func TestGetPeerByNodeIDMissing(t *testing.T) {
	db := testutil.TestDB(t)
	got, err := db.GetPeerByNodeID("missing")
	if err != nil || got != nil {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestGetPeerByAddressMissing(t *testing.T) {
	db := testutil.TestDB(t)
	got, err := db.GetPeerByAddress("http://missing.test")
	if err != nil || got != nil {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestGetPeerMissing(t *testing.T) {
	db := testutil.TestDB(t)
	got, err := db.GetPeer(999)
	if err != nil || got != nil {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestUpsertPeerWithExplicitID(t *testing.T) {
	db := testutil.TestDB(t)
	peer := &models.Peer{
		NodeID: "id-peer", Name: "ID", Address: "http://id.test", PeerSecret: "s",
		Status: models.PeerStatusOnline,
	}
	if err := db.UpsertPeer(peer); err != nil {
		t.Fatal(err)
	}
	peer.Name = "Updated"
	if err := db.UpsertPeer(peer); err != nil {
		t.Fatal(err)
	}
	got, _ := db.GetPeer(peer.ID)
	if got.Name != "Updated" {
		t.Fatalf("name=%q", got.Name)
	}
}

func TestDeletePeerNotFound(t *testing.T) {
	db := testutil.TestDB(t)
	if err := db.DeletePeer(999); err == nil {
		t.Fatal("expected error deleting missing peer")
	}
}

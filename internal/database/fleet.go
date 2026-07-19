package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/hermes-scheduler/hermes/internal/models"
)

func (db *DB) migrateFleet() error {
	schema := `
	CREATE TABLE IF NOT EXISTS peers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		node_id TEXT NOT NULL DEFAULT '',
		name TEXT NOT NULL,
		address TEXT NOT NULL UNIQUE,
		peer_secret TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'unknown',
		last_seen_at DATETIME,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS node_settings (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		node_id TEXT NOT NULL,
		name TEXT NOT NULL DEFAULT '',
		peer_secret TEXT NOT NULL,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_peers_node_id ON peers(node_id);
	CREATE INDEX IF NOT EXISTS idx_peers_status ON peers(status);
	`
	_, err := db.conn.Exec(schema)
	return err
}

func (db *DB) GetNodeSettings() (*models.NodeSettings, error) {
	row := db.conn.QueryRow(`
		SELECT node_id, name, peer_secret, updated_at FROM node_settings WHERE id = 1`)
	var s models.NodeSettings
	err := row.Scan(&s.NodeID, &s.Name, &s.PeerSecret, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (db *DB) SaveNodeSettings(s *models.NodeSettings) error {
	s.UpdatedAt = time.Now().UTC()
	_, err := db.conn.Exec(`
		INSERT INTO node_settings (id, node_id, name, peer_secret, updated_at)
		VALUES (1, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			node_id = excluded.node_id,
			name = excluded.name,
			peer_secret = excluded.peer_secret,
			updated_at = excluded.updated_at`,
		s.NodeID, s.Name, s.PeerSecret, s.UpdatedAt)
	return err
}

func (db *DB) ListPeers() ([]models.Peer, error) {
	rows, err := db.conn.Query(`
		SELECT id, node_id, name, address, peer_secret, status, last_seen_at, created_at
		FROM peers ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var peers []models.Peer
	for rows.Next() {
		p, err := scanPeer(rows)
		if err != nil {
			return nil, err
		}
		peers = append(peers, p)
	}
	return peers, rows.Err()
}

func (db *DB) GetPeer(id int64) (*models.Peer, error) {
	row := db.conn.QueryRow(`
		SELECT id, node_id, name, address, peer_secret, status, last_seen_at, created_at
		FROM peers WHERE id = ?`, id)
	p, err := scanPeerRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (db *DB) GetPeerByNodeID(nodeID string) (*models.Peer, error) {
	row := db.conn.QueryRow(`
		SELECT id, node_id, name, address, peer_secret, status, last_seen_at, created_at
		FROM peers WHERE node_id = ?`, nodeID)
	p, err := scanPeerRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (db *DB) GetPeerByAddress(address string) (*models.Peer, error) {
	row := db.conn.QueryRow(`
		SELECT id, node_id, name, address, peer_secret, status, last_seen_at, created_at
		FROM peers WHERE address = ?`, address)
	p, err := scanPeerRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (db *DB) UpsertPeer(p *models.Peer) error {
	now := time.Now().UTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	if p.Status == "" {
		p.Status = models.PeerStatusUnknown
	}

	if p.ID > 0 {
		_, err := db.conn.Exec(`
			UPDATE peers SET node_id=?, name=?, address=?, peer_secret=?, status=?, last_seen_at=?
			WHERE id=?`,
			p.NodeID, p.Name, p.Address, p.PeerSecret, p.Status, p.LastSeenAt, p.ID)
		return err
	}

	if p.NodeID != "" {
		existing, err := db.GetPeerByNodeID(p.NodeID)
		if err != nil {
			return err
		}
		if existing != nil {
			p.ID = existing.ID
			p.CreatedAt = existing.CreatedAt
			_, err := db.conn.Exec(`
				UPDATE peers SET name=?, address=?, peer_secret=?, status=?, last_seen_at=?
				WHERE id=?`,
				p.Name, p.Address, p.PeerSecret, p.Status, p.LastSeenAt, p.ID)
			if err != nil {
				return err
			}
			_, err = db.conn.Exec(`DELETE FROM peers WHERE node_id = ? AND id != ?`, p.NodeID, p.ID)
			return err
		}
	}

	existing, err := db.GetPeerByAddress(p.Address)
	if err != nil {
		return err
	}
	if existing != nil {
		p.ID = existing.ID
		p.CreatedAt = existing.CreatedAt
		_, err := db.conn.Exec(`
			UPDATE peers SET node_id=?, name=?, peer_secret=?, status=?, last_seen_at=?
			WHERE id=?`,
			p.NodeID, p.Name, p.PeerSecret, p.Status, p.LastSeenAt, p.ID)
		return err
	}

	result, err := db.conn.Exec(`
		INSERT INTO peers (node_id, name, address, peer_secret, status, last_seen_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.NodeID, p.Name, p.Address, p.PeerSecret, p.Status, p.LastSeenAt, p.CreatedAt)
	if err != nil {
		return err
	}
	p.ID, err = result.LastInsertId()
	return err
}

func (db *DB) UpdatePeerStatus(id int64, status models.PeerStatus, lastSeen *time.Time) error {
	_, err := db.conn.Exec(`UPDATE peers SET status=?, last_seen_at=? WHERE id=?`, status, lastSeen, id)
	return err
}

func (db *DB) DeletePeer(id int64) error {
	result, err := db.conn.Exec(`DELETE FROM peers WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("peer not found")
	}
	return nil
}

type peerScanner interface {
	Scan(dest ...interface{}) error
}

func scanPeerFields(s peerScanner) (models.Peer, error) {
	var p models.Peer
	var lastSeen sql.NullTime
	err := s.Scan(&p.ID, &p.NodeID, &p.Name, &p.Address, &p.PeerSecret, &p.Status, &lastSeen, &p.CreatedAt)
	if err != nil {
		return p, err
	}
	if lastSeen.Valid {
		t := lastSeen.Time
		p.LastSeenAt = &t
	}
	return p, nil
}

func scanPeer(rows *sql.Rows) (models.Peer, error) {
	return scanPeerFields(rows)
}

func scanPeerRow(row *sql.Row) (models.Peer, error) {
	return scanPeerFields(row)
}

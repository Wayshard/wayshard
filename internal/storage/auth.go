package storage

import (
	"context"
	"database/sql"
	"time"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/id"
)

func (s *Store) UpsertServerIdentity(ctx context.Context, ident domain.ServerIdentity) error {
	if ident.CreatedAt.IsZero() {
		ident.CreatedAt = time.Now().UTC()
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO server_identity(server_id, public_key, display_name, created_at) VALUES (?,?,?,?)
		ON CONFLICT(server_id) DO UPDATE SET display_name = excluded.display_name`,
		ident.ServerID, ident.PublicKey, ident.DisplayName, ident.CreatedAt.Format(time.RFC3339Nano))
	return err
}

func (s *Store) GetServerIdentity(ctx context.Context) (*domain.ServerIdentity, error) {
	var ident domain.ServerIdentity
	var created string
	err := s.DB.QueryRowContext(ctx, `SELECT server_id, public_key, display_name, created_at FROM server_identity LIMIT 1`).
		Scan(&ident.ServerID, &ident.PublicKey, &ident.DisplayName, &created)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	ident.CreatedAt = parseTime(created)
	return &ident, nil
}

func (s *Store) InsertDevice(ctx context.Context, d *domain.Device) error {
	if d.ID == "" {
		d.ID = id.New()
	}
	now := time.Now().UTC()
	if d.CreatedAt.IsZero() {
		d.CreatedAt = now
	}
	if d.LastSeenAt.IsZero() {
		d.LastSeenAt = now
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO devices(id, name, kind, verifier, salt, pairing_id, created_at, last_seen_at, revoked_at) VALUES (?,?,?,?,?,?,?,?,?)`,
			d.ID, d.Name, d.Kind, d.Verifier, d.Salt, d.PairingID, d.CreatedAt.Format(time.RFC3339Nano), d.LastSeenAt.Format(time.RFC3339Nano), nullTime(d.RevokedAt)); err != nil {
			return err
		}
		_, err := InsertEventJSON(ctx, tx, "device.paired", "", "", "", map[string]any{"id": d.ID, "name": d.Name, "kind": d.Kind})
		return err
	})
}

func (s *Store) GetDevice(ctx context.Context, id string) (*domain.Device, error) {
	var d domain.Device
	var created, seen string
	var revoked sql.NullString
	err := s.DB.QueryRowContext(ctx, `SELECT id, name, kind, verifier, salt, COALESCE(pairing_id,''), created_at, last_seen_at, revoked_at FROM devices WHERE id = ?`, id).
		Scan(&d.ID, &d.Name, &d.Kind, &d.Verifier, &d.Salt, &d.PairingID, &created, &seen, &revoked)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	d.CreatedAt = parseTime(created)
	d.LastSeenAt = parseTime(seen)
	d.RevokedAt = scanNullTime(revoked)
	return &d, nil
}

func (s *Store) ListDevices(ctx context.Context) ([]domain.Device, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, name, kind, verifier, salt, COALESCE(pairing_id,''), created_at, last_seen_at, revoked_at FROM devices ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Device
	for rows.Next() {
		var d domain.Device
		var created, seen string
		var revoked sql.NullString
		if err := rows.Scan(&d.ID, &d.Name, &d.Kind, &d.Verifier, &d.Salt, &d.PairingID, &created, &seen, &revoked); err != nil {
			return nil, err
		}
		d.CreatedAt = parseTime(created)
		d.LastSeenAt = parseTime(seen)
		d.RevokedAt = scanNullTime(revoked)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) RevokeDevice(ctx context.Context, id string) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE devices SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`, nowRFC3339(), id)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return ErrNotFound
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM auth_sessions WHERE device_id = ?`, id); err != nil {
			return err
		}
		_, err = InsertEventJSON(ctx, tx, "device.revoked", "", "", "", map[string]any{"id": id})
		return err
	})
}

func (s *Store) TouchDevice(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE devices SET last_seen_at = ? WHERE id = ?`, nowRFC3339(), id)
	return err
}

func (s *Store) InsertInvitation(ctx context.Context, inv *domain.PairingInvitation) error {
	if inv.ID == "" {
		inv.ID = id.New()
	}
	if inv.CreatedAt.IsZero() {
		inv.CreatedAt = time.Now().UTC()
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO pairing_invitations(id, code_hash, advertised_url, listen_url, server_fingerprint, expires_at, used_at, created_by, created_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		inv.ID, inv.CodeHash, inv.AdvertisedURL, inv.ListenURL, inv.ServerFinger, inv.ExpiresAt.Format(time.RFC3339Nano), nullTime(inv.UsedAt), inv.CreatedBy, inv.CreatedAt.Format(time.RFC3339Nano))
	return err
}

func (s *Store) ListOpenInvitations(ctx context.Context) ([]domain.PairingInvitation, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, code_hash, advertised_url, listen_url, server_fingerprint, expires_at, used_at, created_by, created_at FROM pairing_invitations WHERE used_at IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.PairingInvitation
	for rows.Next() {
		var inv domain.PairingInvitation
		var exp, created string
		var used sql.NullString
		if err := rows.Scan(&inv.ID, &inv.CodeHash, &inv.AdvertisedURL, &inv.ListenURL, &inv.ServerFinger, &exp, &used, &inv.CreatedBy, &created); err != nil {
			return nil, err
		}
		inv.ExpiresAt = parseTime(exp)
		inv.UsedAt = scanNullTime(used)
		inv.CreatedAt = parseTime(created)
		out = append(out, inv)
	}
	return out, rows.Err()
}

func (s *Store) ConsumeInvitation(ctx context.Context, invID string) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE pairing_invitations SET used_at = ? WHERE id = ? AND used_at IS NULL`, nowRFC3339(), invID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrConflict
	}
	return nil
}

func (s *Store) InsertAuthSession(ctx context.Context, sess *domain.AuthSession) error {
	if sess.ID == "" {
		sess.ID = id.New()
	}
	if sess.CreatedAt.IsZero() {
		sess.CreatedAt = time.Now().UTC()
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO auth_sessions(id, device_id, token_hash, expires_at, created_at) VALUES (?,?,?,?,?)`,
		sess.ID, sess.DeviceID, sess.TokenHash, sess.ExpiresAt.Format(time.RFC3339Nano), sess.CreatedAt.Format(time.RFC3339Nano))
	return err
}

func (s *Store) LookupAuthSession(ctx context.Context, tokenHash []byte) (*domain.AuthSession, error) {
	var sess domain.AuthSession
	var exp, created string
	err := s.DB.QueryRowContext(ctx, `SELECT id, device_id, token_hash, expires_at, created_at FROM auth_sessions WHERE token_hash = ?`, tokenHash).
		Scan(&sess.ID, &sess.DeviceID, &sess.TokenHash, &exp, &created)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	sess.ExpiresAt = parseTime(exp)
	sess.CreatedAt = parseTime(created)
	return &sess, nil
}

func (s *Store) DeleteAuthSession(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM auth_sessions WHERE id = ?`, id)
	return err
}

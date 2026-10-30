package store

import (
	"context"
	"database/sql"
	"time"
)

// SupabaseConnection is the metadata view of an owner-scoped Supabase
// connection (migrations/0026). It has no secret field. The encrypted token
// is written by Create and Update and is never selected into this struct.
type SupabaseConnection struct {
	ID         string
	OwnerToken string
	Name       string
	ProjectRef string
	Features   string // JSON array of MCP groups: database, docs, storage
	Management string // JSON array of Management API areas: auth, storage
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

const supabaseConnectionMetaCols = `id, owner_token, name, project_ref, features, management, created_at, updated_at`

func scanSupabaseConnection(sc scanner) (*SupabaseConnection, error) {
	c := &SupabaseConnection{}
	var created, updated int64
	if err := sc.Scan(&c.ID, &c.OwnerToken, &c.Name, &c.ProjectRef, &c.Features, &c.Management, &created, &updated); err != nil {
		return nil, err
	}
	c.CreatedAt = time.Unix(created, 0).UTC()
	c.UpdatedAt = time.Unix(updated, 0).UTC()
	return c, nil
}

// CreateSupabaseConnection inserts an owner-scoped connection. secretEnc and
// secretNonce are the sealed token. ErrConflict when (owner_token, name)
// already exists.
func (s *Store) CreateSupabaseConnection(ctx context.Context, c *SupabaseConnection, secretEnc, secretNonce []byte) error {
	return s.submit(ctx, func(db *sql.DB) error {
		now := time.Now().Unix()
		management := c.Management
		if management == "" {
			management = "[]"
		}
		_, err := db.ExecContext(ctx, `
			INSERT INTO supabase_connection
			  (id, owner_token, name, project_ref, features, management, secret_enc, secret_nonce, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?)`,
			c.ID, c.OwnerToken, c.Name, c.ProjectRef, c.Features, management, secretEnc, secretNonce, now, now)
		if isUniqueViolation(err) {
			return ErrConflict
		}
		if err == nil {
			c.Management = management
			c.CreatedAt = time.Unix(now, 0).UTC()
			c.UpdatedAt = c.CreatedAt
		}
		return err
	})
}

// GetSupabaseConnectionSecret returns the sealed token for an owner's connection.
// The plaintext is never stored. found is false when that owner has no such row.
func (s *Store) GetSupabaseConnectionSecret(ctx context.Context, owner, id string) (secretEnc, secretNonce []byte, found bool, err error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT secret_enc, secret_nonce FROM supabase_connection WHERE owner_token = ? AND id = ?`, owner, id)
	switch err = row.Scan(&secretEnc, &secretNonce); err {
	case nil:
		return secretEnc, secretNonce, true, nil
	case sql.ErrNoRows:
		return nil, nil, false, nil
	default:
		return nil, nil, false, err
	}
}

// GetSupabaseConnection returns one owner's connection as metadata only.
// found is false when no row exists for that owner.
func (s *Store) GetSupabaseConnection(ctx context.Context, owner, id string) (*SupabaseConnection, bool, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+supabaseConnectionMetaCols+` FROM supabase_connection WHERE owner_token = ? AND id = ?`, owner, id)
	c, err := scanSupabaseConnection(row)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return c, true, nil
}

// ListSupabaseConnections returns an owner's connections as metadata only.
func (s *Store) ListSupabaseConnections(ctx context.Context, owner string) ([]*SupabaseConnection, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+supabaseConnectionMetaCols+` FROM supabase_connection WHERE owner_token = ? ORDER BY created_at DESC`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*SupabaseConnection
	for rows.Next() {
		c, err := scanSupabaseConnection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// UpdateSupabaseConnection changes name, project ref, and features for an
// owner's connection. When replaceSecret is true, the sealed token is replaced.
// found is false when no row exists for that owner. ErrConflict when the new
// name is already used by another of the owner's connections.
func (s *Store) UpdateSupabaseConnection(ctx context.Context, owner, id, name, projectRef, features, management string, secretEnc, secretNonce []byte, replaceSecret bool) (*SupabaseConnection, bool, error) {
	var found bool
	err := s.submit(ctx, func(db *sql.DB) error {
		var n int
		if err := db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM supabase_connection WHERE owner_token = ? AND id = ?`, owner, id).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
		found = true
		now := time.Now().Unix()
		var err error
		if management == "" {
			management = "[]"
		}
		if replaceSecret {
			_, err = db.ExecContext(ctx, `
				UPDATE supabase_connection
				   SET name = ?, project_ref = ?, features = ?, management = ?, secret_enc = ?, secret_nonce = ?, updated_at = ?
				 WHERE owner_token = ? AND id = ?`,
				name, projectRef, features, management, secretEnc, secretNonce, now, owner, id)
		} else {
			_, err = db.ExecContext(ctx, `
				UPDATE supabase_connection
				   SET name = ?, project_ref = ?, features = ?, management = ?, updated_at = ?
				 WHERE owner_token = ? AND id = ?`,
				name, projectRef, features, management, now, owner, id)
		}
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return err
	})
	if err != nil || !found {
		return nil, found, err
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT `+supabaseConnectionMetaCols+` FROM supabase_connection WHERE owner_token = ? AND id = ?`, owner, id)
	c, err := scanSupabaseConnection(row)
	return c, true, err
}

// DeleteSupabaseConnection deletes an owner's connection by id. Returns false
// when no such connection exists for that owner.
func (s *Store) DeleteSupabaseConnection(ctx context.Context, owner, id string) (bool, error) {
	var deleted bool
	err := s.submit(ctx, func(db *sql.DB) error {
		if _, err := db.ExecContext(ctx,
			`UPDATE app SET supabase_connection_id = NULL WHERE owner_token = ? AND supabase_connection_id = ?`, owner, id); err != nil {
			return err
		}
		res, err := db.ExecContext(ctx,
			`DELETE FROM supabase_connection WHERE owner_token = ? AND id = ?`, owner, id)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		deleted = n > 0
		return nil
	})
	return deleted, err
}

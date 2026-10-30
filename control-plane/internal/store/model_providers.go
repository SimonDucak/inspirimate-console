package store

import (
	"context"
	"database/sql"
	"errors"
)

// ModelProvider stores public configuration and encrypted credential bytes.
// Validation and encryption belong to the modelprovider service.
type ModelProvider struct {
	ID, Revision, ConfigJSON string
	KeyCiphertext, KeyNonce  []byte
	// Default marks the connection the console preselects. DefaultModel is one
	// of that connection's model IDs. Neither value is a credential.
	Default      bool
	DefaultModel string
}

func scanModelProvider(row scanner) (*ModelProvider, error) {
	p := &ModelProvider{}
	var isDefault int
	err := row.Scan(&p.ID, &p.Revision, &p.ConfigJSON, &p.KeyCiphertext, &p.KeyNonce, &isDefault, &p.DefaultModel)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	p.Default = isDefault != 0
	return p, err
}

const modelProviderColumns = `id, revision, config_json, key_ciphertext, key_nonce, is_default, default_model`

func (s *Store) GetModelProvider(ctx context.Context, id string) (*ModelProvider, error) {
	return scanModelProvider(s.db.QueryRowContext(ctx,
		`SELECT `+modelProviderColumns+` FROM model_provider WHERE id = ?`, id))
}

func (s *Store) ListModelProviders(ctx context.Context) ([]*ModelProvider, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+modelProviderColumns+` FROM model_provider ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*ModelProvider{}
	for rows.Next() {
		p, err := scanModelProvider(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SaveModelProvider replaces the complete connection. A fresh revision revokes
// previously issued task capabilities when the endpoint or credential changes.
func (s *Store) SaveModelProvider(ctx context.Context, p *ModelProvider) error {
	return s.submit(ctx, func(db *sql.DB) error {
		_, err := db.ExecContext(ctx, `INSERT INTO model_provider
			(id, revision, config_json, key_ciphertext, key_nonce) VALUES (?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET revision=excluded.revision,
			config_json=excluded.config_json, key_ciphertext=excluded.key_ciphertext,
			key_nonce=excluded.key_nonce`,
			p.ID, p.Revision, p.ConfigJSON, p.KeyCiphertext, p.KeyNonce)
		return err
	})
}

func (s *Store) DeleteModelProvider(ctx context.Context, id string) error {
	return s.submit(ctx, func(db *sql.DB) error {
		result, err := db.ExecContext(ctx, `DELETE FROM model_provider WHERE id = ?`, id)
		if err != nil {
			return err
		}
		if n, _ := result.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// SetModelProviderDefault makes one connection the instance default and records
// which of its models to preselect. An empty id clears the default. The
// connection revision is left unchanged.
func (s *Store) SetModelProviderDefault(ctx context.Context, id, model string) error {
	return s.submit(ctx, func(db *sql.DB) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, `UPDATE model_provider SET is_default = 0`); err != nil {
			return err
		}
		if id != "" {
			result, err := tx.ExecContext(ctx, `UPDATE model_provider SET is_default = 1, default_model = ? WHERE id = ?`, model, id)
			if err != nil {
				return err
			}
			if n, _ := result.RowsAffected(); n == 0 {
				return ErrNotFound
			}
		}
		return tx.Commit()
	})
}

// RetainDefaultModel keeps a default connection's chosen model when that model
// is still configured. If it was removed, the first remaining model takes its
// place. Connections that are not the default are left unchanged.
func (s *Store) RetainDefaultModel(ctx context.Context, id string, models []string) error {
	return s.submit(ctx, func(db *sql.DB) error {
		var isDefault int
		var current string
		err := db.QueryRowContext(ctx, `SELECT is_default, default_model FROM model_provider WHERE id = ?`, id).Scan(&isDefault, &current)
		if errors.Is(err, sql.ErrNoRows) || isDefault == 0 {
			return nil
		}
		if err != nil {
			return err
		}
		for _, model := range models {
			if model == current {
				return nil
			}
		}
		next := ""
		if len(models) > 0 {
			next = models[0]
		}
		_, err = db.ExecContext(ctx, `UPDATE model_provider SET default_model = ? WHERE id = ?`, next, id)
		return err
	})
}

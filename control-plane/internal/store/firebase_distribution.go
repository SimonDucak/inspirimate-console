package store

import (
	"context"
	"database/sql"
	"time"
)

// FirebaseDistribution is the metadata view of an owner-scoped Firebase App
// Distribution connection. The service account JSON stays in secret_enc and
// is never selected into this struct.
type FirebaseDistribution struct {
	ID           string
	OwnerToken   string
	Name         string
	AppID        string
	ProjectID    string
	ClientEmail  string
	GroupAlias   string
	TesterEmails string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

const firebaseDistributionMetaCols = `id, owner_token, name, app_id, project_id, client_email, group_alias, tester_emails, created_at, updated_at`

func scanFirebaseDistribution(sc scanner) (*FirebaseDistribution, error) {
	c := &FirebaseDistribution{}
	var created, updated int64
	if err := sc.Scan(&c.ID, &c.OwnerToken, &c.Name, &c.AppID, &c.ProjectID, &c.ClientEmail, &c.GroupAlias, &c.TesterEmails, &created, &updated); err != nil {
		return nil, err
	}
	c.CreatedAt = time.Unix(created, 0).UTC()
	c.UpdatedAt = time.Unix(updated, 0).UTC()
	return c, nil
}

// CreateFirebaseDistribution inserts an owner-scoped connection. secretEnc and
// secretNonce are the sealed service account JSON.
func (s *Store) CreateFirebaseDistribution(ctx context.Context, c *FirebaseDistribution, secretEnc, secretNonce []byte) error {
	return s.submit(ctx, func(db *sql.DB) error {
		now := time.Now().Unix()
		emails := c.TesterEmails
		if emails == "" {
			emails = "[]"
		}
		_, err := db.ExecContext(ctx, `
			INSERT INTO firebase_distribution
			  (id, owner_token, name, app_id, project_id, client_email, group_alias, tester_emails, secret_enc, secret_nonce, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
			c.ID, c.OwnerToken, c.Name, c.AppID, c.ProjectID, c.ClientEmail, c.GroupAlias, emails, secretEnc, secretNonce, now, now)
		if isUniqueViolation(err) {
			return ErrConflict
		}
		if err == nil {
			c.TesterEmails = emails
			c.CreatedAt = time.Unix(now, 0).UTC()
			c.UpdatedAt = c.CreatedAt
		}
		return err
	})
}

// GetFirebaseDistributionSecret returns the sealed service account for an
// owner's connection. found is false when that owner has no such row.
func (s *Store) GetFirebaseDistributionSecret(ctx context.Context, owner, id string) (secretEnc, secretNonce []byte, found bool, err error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT secret_enc, secret_nonce FROM firebase_distribution WHERE owner_token = ? AND id = ?`, owner, id)
	switch err = row.Scan(&secretEnc, &secretNonce); err {
	case nil:
		return secretEnc, secretNonce, true, nil
	case sql.ErrNoRows:
		return nil, nil, false, nil
	default:
		return nil, nil, false, err
	}
}

// GetFirebaseDistribution returns metadata for an owner's connection.
func (s *Store) GetFirebaseDistribution(ctx context.Context, owner, id string) (*FirebaseDistribution, bool, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+firebaseDistributionMetaCols+` FROM firebase_distribution WHERE owner_token = ? AND id = ?`, owner, id)
	c, err := scanFirebaseDistribution(row)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return c, true, nil
}

// ListFirebaseDistributions returns an owner's connections as metadata only.
func (s *Store) ListFirebaseDistributions(ctx context.Context, owner string) ([]*FirebaseDistribution, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+firebaseDistributionMetaCols+` FROM firebase_distribution WHERE owner_token = ? ORDER BY created_at DESC`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*FirebaseDistribution
	for rows.Next() {
		c, err := scanFirebaseDistribution(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// UpdateFirebaseDistribution changes the public fields. When replaceSecret is
// true, the sealed service account is replaced. found is false when no row
// exists for that owner.
func (s *Store) UpdateFirebaseDistribution(ctx context.Context, owner, id string, c *FirebaseDistribution, secretEnc, secretNonce []byte, replaceSecret bool) (*FirebaseDistribution, bool, error) {
	var found bool
	err := s.submit(ctx, func(db *sql.DB) error {
		var n int
		if err := db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM firebase_distribution WHERE owner_token = ? AND id = ?`, owner, id).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
		found = true
		now := time.Now().Unix()
		emails := c.TesterEmails
		if emails == "" {
			emails = "[]"
		}
		var err error
		if replaceSecret {
			_, err = db.ExecContext(ctx, `
				UPDATE firebase_distribution
				   SET name = ?, app_id = ?, project_id = ?, client_email = ?, group_alias = ?, tester_emails = ?, secret_enc = ?, secret_nonce = ?, updated_at = ?
				 WHERE owner_token = ? AND id = ?`,
				c.Name, c.AppID, c.ProjectID, c.ClientEmail, c.GroupAlias, emails, secretEnc, secretNonce, now, owner, id)
		} else {
			_, err = db.ExecContext(ctx, `
				UPDATE firebase_distribution
				   SET name = ?, app_id = ?, group_alias = ?, tester_emails = ?, updated_at = ?
				 WHERE owner_token = ? AND id = ?`,
				c.Name, c.AppID, c.GroupAlias, emails, now, owner, id)
		}
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return err
	})
	if err != nil || !found {
		return nil, found, err
	}
	return s.GetFirebaseDistribution(ctx, owner, id)
}

// DeleteFirebaseDistribution deletes an owner's connection and clears it from
// any app that pointed at it.
func (s *Store) DeleteFirebaseDistribution(ctx context.Context, owner, id string) (bool, error) {
	var deleted bool
	err := s.submit(ctx, func(db *sql.DB) error {
		if _, err := db.ExecContext(ctx,
			`UPDATE app SET firebase_distribution_id = NULL WHERE owner_token = ? AND firebase_distribution_id = ?`, owner, id); err != nil {
			return err
		}
		res, err := db.ExecContext(ctx,
			`DELETE FROM firebase_distribution WHERE owner_token = ? AND id = ?`, owner, id)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		deleted = n > 0
		return nil
	})
	return deleted, err
}

package store

import "fmt"

// ListStarredCompanies returns the slugs of every starred company,
// alphabetically. The slice is initialized (never nil) so the Wails JSON
// boundary yields [] rather than null.
func (db *DB) ListStarredCompanies() ([]string, error) {
	rows, err := db.conn.Query(`SELECT slug FROM starred_companies ORDER BY slug`)
	if err != nil {
		return nil, fmt.Errorf("store: list starred companies: %w", err)
	}
	defer rows.Close()

	slugs := []string{}
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, fmt.Errorf("store: scan starred company: %w", err)
		}
		slugs = append(slugs, slug)
	}
	return slugs, rows.Err()
}

// SetCompanyStarred stars (true) or unstars (false) a company by slug. Both
// directions are idempotent, so repeated toggles never error.
func (db *DB) SetCompanyStarred(slug string, starred bool) error {
	if starred {
		if _, err := db.conn.Exec(`INSERT OR IGNORE INTO starred_companies (slug) VALUES (?)`, slug); err != nil {
			return fmt.Errorf("store: star company %q: %w", slug, err)
		}
		return nil
	}
	if _, err := db.conn.Exec(`DELETE FROM starred_companies WHERE slug = ?`, slug); err != nil {
		return fmt.Errorf("store: unstar company %q: %w", slug, err)
	}
	return nil
}

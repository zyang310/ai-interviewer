package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

// DB wraps a SQLite connection.
type DB struct {
	conn *sql.DB
}

// Open creates the application data directory if needed, opens the SQLite
// database, enables WAL mode, and runs the schema migrations.
func Open() (*DB, error) {
	dir, err := appDataDir()
	if err != nil {
		return nil, fmt.Errorf("store: resolve data dir: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("store: create data dir: %w", err)
	}

	path := filepath.Join(dir, "data.db")
	conn, err := sql.Open("sqlite3", path+"?_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("store: open db: %w", err)
	}

	// SQLite works best with a single writer connection.
	conn.SetMaxOpenConns(1)

	if _, err := conn.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		return nil, fmt.Errorf("store: enable WAL: %w", err)
	}

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("store: migrate: %w", err)
	}
	return db, nil
}

// Close shuts down the database connection.
func (db *DB) Close() error {
	return db.conn.Close()
}

// migrate creates tables that do not yet exist. Add new statements here as
// the schema evolves — existing tables are left untouched.
func (db *DB) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			id         TEXT PRIMARY KEY,
			problem_id TEXT NOT NULL,
			model      TEXT NOT NULL,
			started_at DATETIME NOT NULL,
			ended_at   DATETIME
		);`,
		`CREATE TABLE IF NOT EXISTS messages (
			id         TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			role       TEXT NOT NULL,
			content    TEXT NOT NULL,
			has_image  INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL,
			FOREIGN KEY (session_id) REFERENCES sessions(id)
		);`,
		`CREATE TABLE IF NOT EXISTS preferences (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);`,
	}
	for _, s := range stmts {
		if _, err := db.conn.Exec(s); err != nil {
			return err
		}
	}
	return nil
}

// appDataDir returns ~/Library/Application Support/ai-interviewer on macOS.
func appDataDir() (string, error) {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfg, "ai-interviewer"), nil
}

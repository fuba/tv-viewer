package db

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
)

func Init(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	// Create tables
	if err := createTables(db); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

func createTables(db *sql.DB) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS channels (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			channel TEXT NOT NULL,
			name TEXT NOT NULL,
			services TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS programs (
			id INTEGER PRIMARY KEY,
			event_id INTEGER NOT NULL,
			service_id INTEGER NOT NULL,
			network_id INTEGER NOT NULL,
			start_at DATETIME NOT NULL,
			duration INTEGER NOT NULL,
			name TEXT NOT NULL,
			description TEXT,
			genre TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_programs_start_at ON programs(start_at)`,
		`CREATE INDEX IF NOT EXISTS idx_programs_service_id ON programs(service_id)`,
	}

	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			return err
		}
	}

	return nil
}
package params

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// Store provides SQLite-backed persistent parameter storage
type Store struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewStore creates a new parameter store, initializes the schema, and seeds default rows.
// Use ":memory:" as dbPath for an in-memory database (useful for testing).
func NewStore(dbPath string, logger *slog.Logger) (*Store, error) {
	// Ensure the directory exists (skip for in-memory)
	if dbPath != ":memory:" {
		dir := filepath.Dir(dbPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	// Verify the connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite database: %w", err)
	}

	s := &Store{
		db:     db,
		logger: logger,
	}

	if err := s.createTable(); err != nil {
		db.Close()
		return nil, fmt.Errorf("create parameters table: %w", err)
	}

	if err := s.seedDefaults(); err != nil {
		db.Close()
		return nil, fmt.Errorf("seed default parameters: %w", err)
	}

	logger.Info("Parameter store initialized", "path", dbPath)
	return s, nil
}

// createTable creates the parameters table if it does not exist
func (s *Store) createTable() error {
	query := `
	CREATE TABLE IF NOT EXISTS parameters (
		id          TEXT PRIMARY KEY,
		key         TEXT UNIQUE NOT NULL,
		description TEXT,
		content     TEXT,
		changed     TIMESTAMP NOT NULL
	)`
	_, err := s.db.Exec(query)
	return err
}

// seedDefaults inserts default parameters if they don't already exist
func (s *Store) seedDefaults() error {
	query := `INSERT OR IGNORE INTO parameters (id, key, description, content, changed) VALUES (?, ?, ?, ?, ?)`
	for _, p := range DefaultParams {
		id := uuid.New().String()
		now := time.Now()
		_, err := s.db.Exec(query, id, p.Key, p.Description, p.Content, now)
		if err != nil {
			return fmt.Errorf("seed parameter %q: %w", p.Key, err)
		}
	}
	s.logger.Info("Default parameters seeded")
	return nil
}

// GetAll returns all persistent parameters
func (s *Store) GetAll() ([]PersistentParam, error) {
	rows, err := s.db.Query("SELECT id, key, description, content, changed FROM parameters ORDER BY key")
	if err != nil {
		return nil, fmt.Errorf("query parameters: %w", err)
	}
	defer rows.Close()

	var params []PersistentParam
	for rows.Next() {
		var p PersistentParam
		if err := rows.Scan(&p.ID, &p.Key, &p.Description, &p.Content, &p.Changed); err != nil {
			return nil, fmt.Errorf("scan parameter: %w", err)
		}
		params = append(params, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate parameters: %w", err)
	}

	return params, nil
}

// GetByKey returns a single parameter by its key
func (s *Store) GetByKey(key string) (*PersistentParam, error) {
	var p PersistentParam
	err := s.db.QueryRow(
		"SELECT id, key, description, content, changed FROM parameters WHERE key = ?", key,
	).Scan(&p.ID, &p.Key, &p.Description, &p.Content, &p.Changed)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query parameter by key: %w", err)
	}
	return &p, nil
}

// Create inserts a new parameter. Returns ErrDuplicateKey if the key already exists.
func (s *Store) Create(req CreateParamRequest) (*PersistentParam, error) {
	p := PersistentParam{
		ID:          uuid.New().String(),
		Key:         req.Key,
		Description: req.Description,
		Content:     req.Content,
		Changed:     time.Now(),
	}

	_, err := s.db.Exec(
		"INSERT INTO parameters (id, key, description, content, changed) VALUES (?, ?, ?, ?, ?)",
		p.ID, p.Key, p.Description, p.Content, p.Changed,
	)
	if err != nil {
		// SQLite UNIQUE constraint violation
		if isUniqueViolation(err) {
			return nil, ErrDuplicateKey
		}
		return nil, fmt.Errorf("insert parameter: %w", err)
	}

	return &p, nil
}

// Update updates an existing parameter by key. Returns ErrNotFound if the key does not exist.
func (s *Store) Update(key string, req UpdateParamRequest) (*PersistentParam, error) {
	now := time.Now()

	result, err := s.db.Exec(
		"UPDATE parameters SET description = ?, content = ?, changed = ? WHERE key = ?",
		req.Description, req.Content, now, key,
	)
	if err != nil {
		return nil, fmt.Errorf("update parameter: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return nil, ErrNotFound
	}

	// Return the updated parameter
	return s.GetByKey(key)
}

// Close closes the database connection
func (s *Store) Close() error {
	return s.db.Close()
}

// isUniqueViolation checks if the error is a SQLite UNIQUE constraint violation
func isUniqueViolation(err error) bool {
	return err != nil && contains(err.Error(), "UNIQUE constraint failed")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

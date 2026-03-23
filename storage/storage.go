package storage

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"

	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

// Entry represents a single saved string in the history
type Entry struct {
	ID        int
	Content   string
	CreatedAt time.Time
	IPAddress string
}

// Store handles database interactions
type Store struct {
	db     *sql.DB
	dbType string
}

// NewStore initializes the database and returns a Store instance
func NewStore(dbType, dbName string) (*Store, error) {
	switch dbType {
	case "sqlite":
		// Create the directory if it doesn't exist for SQLite
		dir := filepath.Dir(dbName)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}

	db, err := sql.Open(dbType, dbName)
	if err != nil {
		return nil, err
	}

	var createTableSQL string
	switch dbType {
	case "sqlite":
		db.SetMaxOpenConns(1)
		enableWALSQL := "PRAGMA journal_mode=WAL;"
		if _, err := db.Exec(enableWALSQL); err != nil {
			db.Close()
			return nil, err
		}
		createTableSQL = `CREATE TABLE IF NOT EXISTS entries (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			content TEXT,
			created_at DATETIME,
			ip_address TEXT
		);`
	case "postgres":
		if err := db.Ping(); err != nil {
			db.Close()
			return nil, err
		}
		createTableSQL = `CREATE TABLE IF NOT EXISTS entries (
			id SERIAL PRIMARY KEY,
			content TEXT,
			created_at TIMESTAMP,
			ip_address TEXT
		);`
	}

	if _, err := db.Exec(createTableSQL); err != nil {
		db.Close()
		return nil, err
	}

	return &Store{db: db, dbType: dbType}, nil
}

// Close closes the database connection
func (s *Store) Close() error {
	return s.db.Close()
}

// AddEntry inserts a new entry into the database
func (s *Store) AddEntry(content, ip string) (int, error) {
	switch s.dbType {
	case "postgres":
		query := "INSERT INTO entries(content, created_at, ip_address) VALUES($1, $2, $3) RETURNING id"
		var id int
		err := s.db.QueryRow(query, content, time.Now().UTC(), ip).Scan(&id)
		if err != nil {
			return 0, err
		}
		return id, nil
	default:
		query := "INSERT INTO entries(content, created_at, ip_address) VALUES(?, ?, ?)"
		stmt, err := s.db.Prepare(query)
		if err != nil {
			return 0, err
		}
		defer stmt.Close()

		res, err := stmt.Exec(content, time.Now().UTC(), ip)
		if err != nil {
			return 0, err
		}

		id, err := res.LastInsertId()
		if err != nil {
			return 0, err
		}
		return int(id), nil
	}
}

func (s *Store) GetEntriesPaged(limit, offset int) ([]Entry, error) {
	query := "SELECT id, content, created_at, ip_address FROM entries ORDER BY id DESC LIMIT ? OFFSET ?"
	switch s.dbType {
	case "postgres":
		query = "SELECT id, content, created_at, ip_address FROM entries ORDER BY id DESC LIMIT $1 OFFSET $2"
	}
	rows, err := s.db.Query(query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.Content, &e.CreatedAt, &e.IPAddress); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, nil
}

func (s *Store) GetTotalCount() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM entries").Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// GetEntry retrieves a specific entry by ID
func (s *Store) GetEntry(id int) (*Entry, error) {
	var e Entry
	query := "SELECT id, content, created_at, ip_address FROM entries WHERE id = ?"
	switch s.dbType {
	case "postgres":
		query = "SELECT id, content, created_at, ip_address FROM entries WHERE id = $1"
	}
	err := s.db.QueryRow(query, id).
		Scan(&e.ID, &e.Content, &e.CreatedAt, &e.IPAddress)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// GetLatestID returns the ID of the most recent entry
func (s *Store) GetLatestID() (int, error) {
	var id sql.NullInt64
	err := s.db.QueryRow("SELECT MAX(id) FROM entries").Scan(&id)
	if err != nil {
		return 0, err
	}
	// If empty
	if !id.Valid {
		return 0, nil
	}
	return int(id.Int64), nil
}

func (s *Store) RemoveEntry(id int) error {
	query := "DELETE FROM entries WHERE id = ?"
	switch s.dbType {
	case "postgres":
		query = "DELETE FROM entries WHERE id = $1"
	}
	_, err := s.db.Exec(query, id)
	if err != nil {
		return err
	}
	return nil
}

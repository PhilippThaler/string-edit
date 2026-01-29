package storage

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"

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
	db *sql.DB
}

// NewStore initializes the SQLite database and returns a Store instance
func NewStore(dbPath string) (*Store, error) {
	// Create the directory if it doesn't exist
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	// Driver name is "sqlite" for modernc.org/sqlite
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	createTableSQL := `CREATE TABLE IF NOT EXISTS entries (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		content TEXT,
		created_at DATETIME,
		ip_address TEXT
	);`

	if _, err := db.Exec(createTableSQL); err != nil {
		db.Close()
		return nil, err
	}

	return &Store{db: db}, nil
}

// Close closes the database connection
func (s *Store) Close() error {
	return s.db.Close()
}

// AddEntry inserts a new entry into the database
func (s *Store) AddEntry(content, ip string) (int, error) {
	stmt, err := s.db.Prepare("INSERT INTO entries(content, created_at, ip_address) VALUES(?, ?, ?)")
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	res, err := stmt.Exec(content, time.Now(), ip)
	if err != nil {
		return 0, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

// GetEntry retrieves a specific entry by ID
func (s *Store) GetEntry(id int) (*Entry, error) {
	var e Entry
	err := s.db.QueryRow("SELECT id, content, created_at, ip_address FROM entries WHERE id = ?", id).
		Scan(&e.ID, &e.Content, &e.CreatedAt, &e.IPAddress)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// GetLatestID returns the ID of the most recent entry
func (s *Store) GetLatestID() (int, error) {
	var id int
	// Returns error if table is empty (Scan fails on NULL)
	err := s.db.QueryRow("SELECT MAX(id) FROM entries").Scan(&id)
	if err != nil {
		return 0, nil // Return 0 if empty or error, caller handles "empty db" case
	}
	return id, nil
}

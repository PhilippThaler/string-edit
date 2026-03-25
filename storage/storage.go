package storage

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"

	sq "github.com/Masterminds/squirrel"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

// Entry represents a single saved string in the history
type Entry struct {
	ID        int
	Content   string
	CreatedAt time.Time
	IPAddress string
	IsDeleted bool
}

// Store handles database interactions
type Store struct {
	db      *sql.DB
	dbType  string
	builder sq.StatementBuilderType
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

	var builder sq.StatementBuilderType
	if dbType == "postgres" {
		builder = sq.StatementBuilder.PlaceholderFormat(sq.Dollar).RunWith(db)
	} else {
		builder = sq.StatementBuilder.PlaceholderFormat(sq.Question).RunWith(db)
	}

	var createTableSQL string
	var createActiveViewSQL string
	var createDeletedIndexSQL string
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
			ip_address TEXT,
			is_deleted BOOLEAN DEFAULT false
		);`
		createActiveViewSQL = `
			CREATE VIEW IF NOT EXISTS active_entries AS 
				SELECT id, content, created_at, ip_address 
				FROM entries
				WHERE is_deleted = false;
	`
		createDeletedIndexSQL = `
			CREATE INDEX IF NOT EXISTS idx_active_entries 
				ON entries (created_at DESC, id) 
				WHERE is_deleted = false;
		`
	case "postgres":
		if err := db.Ping(); err != nil {
			db.Close()
			return nil, err
		}
		createTableSQL = `CREATE TABLE IF NOT EXISTS entries (
			id SERIAL PRIMARY KEY,
			content TEXT,
			created_at TIMESTAMP,
			ip_address TEXT,
			is_deleted BOOLEAN DEFAULT false
		);`
		createActiveViewSQL = `
			CREATE OR REPLACE VIEW active_entries AS 
				SELECT id, content, created_at, ip_address 
				FROM entries
				WHERE is_deleted = false;
	`
		createDeletedIndexSQL = `
			CREATE INDEX IF NOT EXISTS idx_active_entries 
				ON entries (created_at DESC, id) 
				WHERE is_deleted = false;
		`
	}

	if _, err := db.Exec(createTableSQL); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(createActiveViewSQL); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(createDeletedIndexSQL); err != nil {
		db.Close()
		return nil, err
	}

	return &Store{
		db:      db,
		dbType:  dbType,
		builder: builder,
	}, nil
}

// Close closes the database connection
func (s *Store) Close() error {
	return s.db.Close()
}

// AddEntry inserts a new entry into the database
func (s *Store) AddEntry(content, ip string) (int, error) {
	query := s.builder.Insert("entries").
		Columns("content", "created_at", "ip_address").
		Values(content, time.Now().UTC(), ip)

	if s.dbType == "postgres" {
		var id int
		err := query.Suffix("RETURNING id").QueryRow().Scan(&id)
		return id, err
	}

	res, err := query.Exec()
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	return int(id), err
}

func (s *Store) GetActiveEntriesPaged(limit, offset int) ([]Entry, error) {
	rows, err := s.builder.Select("id, content, created_at, ip_address").
		From("active_entries").
		OrderBy("id DESC").
		Limit(uint64(limit)).
		Offset(uint64(offset)).Query()

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

	err := s.builder.Select("COUNT(*)").
		From("entries").
		QueryRow().
		Scan(&count)

	if err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) GetTotalActiveCount() (int, error) {
	var count int

	err := s.builder.Select("COUNT(*)").
		From("active_entries").
		QueryRow().
		Scan(&count)

	if err != nil {
		return 0, err
	}
	return count, nil
}

// GetEntry retrieves a specific entry by ID
func (s *Store) GetEntry(id int) (*Entry, error) {
	var e Entry

	err := s.builder.Select("id", "content", "created_at", "ip_address", "is_deleted").
		From("entries").
		Where(sq.Eq{"id": id}).
		QueryRow().
		Scan(&e.ID, &e.Content, &e.CreatedAt, &e.IPAddress, &e.IsDeleted)

	if err != nil {
		return nil, err
	}
	return &e, nil
}

// GetLatestID returns the ID of the most recent entry
func (s *Store) GetLatestID() (int, error) {
	var id sql.NullInt64

	err := s.builder.Select("MAX(id)").
		From("entries").
		QueryRow().
		Scan(&id)

	if err != nil {
		return 0, err
	}
	// If empty
	if !id.Valid {
		return 0, nil
	}
	return int(id.Int64), nil
}

// GetLatestID returns the ID of the most recent entry
func (s *Store) GetLatestActiveID() (int, error) {
	var id sql.NullInt64

	err := s.builder.Select("MAX(id)").
		From("active_entries").
		QueryRow().
		Scan(&id)

	if err != nil {
		return 0, err
	}
	// If empty
	if !id.Valid {
		return 0, nil
	}
	return int(id.Int64), nil
}

func (s *Store) GetPrevID(currentID int, currentTimestamp time.Time) (int, error) {
	var prevID int

	err := s.builder.Select("id").
		From("active_entries").
		Where(sq.Expr("(created_at, id) < (?, ?)", currentTimestamp, currentID)).
		OrderBy("created_at DESC", "id DESC").
		Limit(1).
		QueryRow().
		Scan(&prevID)

	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return prevID, nil
}

func (s *Store) GetNextID(currentID int, currentTimestamp time.Time) (int, error) {
	var nextID int

	err := s.builder.Select("id").
		From("active_entries").
		Where(sq.Expr("(created_at, id) > (?, ?)", currentTimestamp, currentID)).
		OrderBy("created_at ASC", "id ASC").
		Limit(1).
		QueryRow().
		Scan(&nextID)

	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return nextID, nil
}

func (s *Store) RemoveEntry(id int) error {
	_, err := s.builder.Update("entries").
		Set("is_deleted", true).
		Where(sq.Eq{"id": id}).
		Exec()

	if err != nil {
		return err
	}
	return nil
}

package storage

import (
	"os"
	"testing"
)

func TestPagination(t *testing.T) {
	dbPath := "test_pagination.db"
	defer os.Remove(dbPath)

	store, err := NewStore("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Add 25 entries
	for i := 1; i <= 25; i++ {
		_, err := store.AddEntry("entry", "127.0.0.1")
		if err != nil {
			t.Fatalf("failed to add entry %d: %v", i, err)
		}
	}

	count, err := store.GetTotalCount()
	if err != nil {
		t.Fatalf("failed to get count: %v", err)
	}
	if count != 25 {
		t.Errorf("expected 25 entries, got %d", count)
	}

	// Test page 1 (limit 10, offset 0)
	entries, err := store.GetEntriesPaged(10, 0)
	if err != nil {
		t.Fatalf("failed to get page 1: %v", err)
	}
	if len(entries) != 10 {
		t.Errorf("expected 10 entries on page 1, got %d", len(entries))
	}
	if entries[0].ID != 25 {
		t.Errorf("expected newest entry (ID 25) first, got %d", entries[0].ID)
	}

	// Test page 3 (limit 10, offset 20)
	entries, err = store.GetEntriesPaged(10, 20)
	if err != nil {
		t.Fatalf("failed to get page 3: %v", err)
	}
	if len(entries) != 5 {
		t.Errorf("expected 5 entries on page 3, got %d", len(entries))
	}
	if entries[0].ID != 5 {
		t.Errorf("expected entry ID 5 first on page 3, got %d", entries[0].ID)
	}
}

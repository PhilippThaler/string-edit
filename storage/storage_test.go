package storage

import (
	"os"
	"testing"
)

func setupTestDB(t *testing.T) *Store {
	dbPath := "test.db"

	store, err := NewStore("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	t.Cleanup(func() {
		store.Close()
		os.Remove(dbPath)
	})

	return store
}

func addEntries(num int, store *Store, t *testing.T) {
	for i := 1; i <= num; i++ {
		_, err := store.AddEntry("entry", "127.0.0.1")
		if err != nil {
			t.Fatalf("failed to add entry %d: %v", i, err)
		}
	}

	count, err := store.GetTotalCount()
	if err != nil {
		t.Fatalf("failed to get count: %v", err)
	}
	if count != num {
		t.Errorf("expected %d entries, got %d", num, count)
	}
}

func testPrevID(currentID, expectedPrevID int, store *Store, t *testing.T) {
	entry, err := store.GetEntry(currentID)
	if err != nil {
		t.Fatalf("failed to get entry: %v", err)
	}
	prevID, err := store.GetPrevID(entry.ID, entry.CreatedAt.String())
	if err != nil {
		t.Fatalf("failed to get prevID: %v", err)
	}
	if prevID != expectedPrevID {
		t.Errorf("expected prevID %d, got %d", expectedPrevID, prevID)
	}
}

func testNextID(currentID, expectedNextID int, store *Store, t *testing.T) {
	entry, err := store.GetEntry(currentID)
	if err != nil {
		t.Fatalf("failed to get entry: %v", err)
	}
	nextID, err := store.GetNextID(entry.ID, entry.CreatedAt.String())
	if err != nil {
		t.Fatalf("failed to get prevID: %v", err)
	}
	if nextID != expectedNextID {
		t.Errorf("expected nextID %d, got %d", expectedNextID, nextID)
	}
}

func TestNavigation(t *testing.T) {
	store := setupTestDB(t)

	const (
		numEntries = 10
	)

	addEntries(numEntries, store, t)

	// When there's no next or prev, return 0
	currentID := 1
	testPrevID(currentID, 0, store, t)
	currentID = numEntries
	testNextID(currentID, 0, store, t)

	currentID = numEntries - 1
	store.RemoveEntry(numEntries)
	testNextID(currentID, 0, store, t)

	currentID = 2
	store.RemoveEntry(currentID - 1)
	testPrevID(currentID, 0, store, t)
	testNextID(currentID, currentID+1, store, t)
	currentID = 4
	store.RemoveEntry(currentID + 1)
	store.RemoveEntry(currentID - 1)
	testPrevID(currentID, currentID-2, store, t)
	testNextID(currentID, currentID+2, store, t)

}

func TestSoftDelete(t *testing.T) {
	store := setupTestDB(t)

	const (
		numEntries     = 10
		deletedEntries = 3
	)

	addEntries(numEntries, store, t)

	for i := range deletedEntries {
		store.RemoveEntry(i + 1)
	}
	totalCount, err := store.GetTotalCount()
	if err != nil {
		t.Fatalf("failed to get count: %v", err)
	}
	if totalCount != numEntries {
		t.Errorf("expected %d entries, got %d", totalCount, numEntries)
	}

	activeCount, err := store.GetTotalActiveCount()
	if err != nil {
		t.Fatalf("failed to get count: %v", err)
	}
	if activeCount != totalCount-deletedEntries {
		t.Errorf("expected %d entries, got %d", totalCount-deletedEntries, activeCount)
	}

	latestActiveID, err := store.GetLatestActiveID()
	if err != nil {
		t.Fatalf("failed to get latest active id: %v", err)
	}
	if latestActiveID != totalCount {
		t.Errorf("expected %d, got %d", totalCount, latestActiveID)
	}

	// delete last entry
	store.RemoveEntry(latestActiveID)

	latestActiveID, err = store.GetLatestActiveID()
	if err != nil {
		t.Fatalf("failed to get latest active id: %v", err)
	}
	if latestActiveID != totalCount-1 {
		t.Errorf("expected %d, got %d", totalCount-1, latestActiveID)
	}

	entries, err := store.GetActiveEntriesPaged(totalCount, 0)
	if err != nil {
		t.Fatalf("failed to get active entries: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDeleted {
			t.Errorf("GetActiveEntriesPaged shouldn't return a deleted entry")
		}
	}
}

func TestPagination(t *testing.T) {
	store := setupTestDB(t)

	const (
		numEntries = 25
		pageSize   = 10
	)

	addEntries(numEntries, store, t)

	// Test page 1 (limit 10, offset 0)
	entries, err := store.GetActiveEntriesPaged(pageSize, 0)
	if err != nil {
		t.Fatalf("failed to get page 1: %v", err)
	}
	if len(entries) != pageSize {
		t.Errorf("expected 10 entries on page 1, got %d", len(entries))
	}
	if entries[0].ID != numEntries {
		t.Errorf("expected newest entry (ID 25) first, got %d", entries[0].ID)
	}

	// Test page 3 (limit 10, offset 20)
	entries, err = store.GetActiveEntriesPaged(pageSize, 20)
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

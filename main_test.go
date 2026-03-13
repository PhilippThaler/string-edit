package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"site/storage"
	"testing"
	"time"
)

func TestListPagination(t *testing.T) {
	dbPath := "test_list.db"
	defer os.Remove(dbPath)

	store, err := storage.NewStore("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Add 15 entries
	for i := 1; i <= 15; i++ {
		_, err := store.AddEntry("entry", "127.0.0.1")
		if err != nil {
			t.Fatalf("failed to add entry %d: %v", i, err)
		}
	}

	loc := time.UTC
	mux := newServer(store, loc)

	// Test page 1
	req := httptest.NewRequest("GET", "/list?page=1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status OK, got %v", rr.Code)
	}

	// Test page 2
	req = httptest.NewRequest("GET", "/list?page=2", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status OK, got %v", rr.Code)
	}
}

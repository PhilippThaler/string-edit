package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"site/storage"
	"site/view"
)

const (
	defaultDBPath = "data/history.db"

	pageSize                = 30
	maxLen                  = 500
	defaultDBEntryText      = "Hello, World!"
	defaultDBEntryIPAddress = "system"
)

func run() error {
	store, err := storage.NewStore(defaultDBPath)
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	defer store.Close()

	// Load Timezone
	tz := os.Getenv("TZ")
	if tz == "" {
		tz = "UTC"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		slog.Warn("Invalid timezone, defaulting to UTC", "timezone", tz, "error", err)
		loc = time.UTC
	}

	if latest, err := store.GetLatestID(); err != nil {
		return fmt.Errorf("failed to get latest ID: %w", err)
	} else if latest == 0 {
		// If database is empty, create first entry
		if _, err := store.AddEntry(defaultDBEntryText, defaultDBEntryIPAddress); err != nil {
			return fmt.Errorf("failed to add initial entry: %w", err)
		}
	}

	mux := newServer(store, loc)

	srv := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	// Start server in a goroutine
	go func() {
		slog.Info("Server started at http://localhost:8080")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("listen error", "error", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server with a timeout of 5 seconds.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	slog.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("server forced to shutdown: %w", err)
	}

	slog.Info("Server exiting")
	return nil
}

func newServer(store *storage.Store, loc *time.Location) http.Handler {
	mux := http.NewServeMux()

	// Redirect root to the newest entry
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		latest, err := store.GetLatestID()
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			slog.Error("Could not get latest ID", "error", err)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/%d", latest), http.StatusFound)
	})

	// Show form for a new entry
	mux.HandleFunc("GET /new", func(w http.ResponseWriter, r *http.Request) {
		err := view.New().Render(r.Context(), w)
		if err != nil {
			slog.Error("Template execution error", "error", err)
		}
	})

	// View a specific entry
	mux.HandleFunc("GET /{id}", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		latest, err := store.GetLatestID()
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			slog.Error("Could not get latest ID", "error", err)
			return
		}
		if id < 1 || id > latest {
			http.NotFound(w, r)
			return
		}

		entry, err := store.GetEntry(id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, "Error retrieving entry", http.StatusInternalServerError)
			return
		}

		prevLink := ""
		if id > 1 {
			prevLink = fmt.Sprintf("/%d", id-1)
		}

		nextLink := ""
		if id < latest {
			nextLink = fmt.Sprintf("/%d", id+1)
		}

		err = view.View(
			entry.Content,
			entry.CreatedAt.In(loc).Format("Jan 02, 2006 15:04:05 MST"),
			prevLink,
			nextLink,
			"/new",
			id,
			latest,
		).Render(r.Context(), w)

		if err != nil {
			slog.Error("Template execution error", "error", err)
		}
	})

	mux.HandleFunc("GET /list", func(w http.ResponseWriter, r *http.Request) {
		pageStr := r.URL.Query().Get("page")
		page, err := strconv.Atoi(pageStr)
		if err != nil || page < 1 {
			page = 1
		}

		totalEntries, err := store.GetTotalCount()
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			slog.Error("Could not get total entries", "error", err)
			return
		}

		totalPages := (totalEntries + pageSize - 1) / pageSize
		if totalPages == 0 {
			totalPages = 1
		}

		if page > totalPages {
			page = totalPages
		}

		offset := (page - 1) * pageSize
		entries, err := store.GetEntriesPaged(pageSize, offset)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			slog.Error("Could not get entries", "error", err)
			return
		}

		err = view.List(entries, page, totalPages).Render(r.Context(), w)
		if err != nil {
			slog.Error("Template execution error", "error", err)
		}
	})

	// Save a new entry
	mux.HandleFunc("POST /save", func(w http.ResponseWriter, r *http.Request) {
		newText := strings.TrimSpace(r.FormValue("newText"))

		if newText == "" {
			http.Error(w, "Content cannot be empty", http.StatusBadRequest)
			return
		}
		if utf8.RuneCountInString(newText) > maxLen {
			http.Error(w, fmt.Sprintf("Content too long (max %d chars)", maxLen), http.StatusBadRequest)
			return
		}

		// Get IP address (Handle Reverse Proxy headers)
		ip := r.Header.Get("X-Forwarded-For")
		if ip != "" {
			// If multiple proxies, the first IP is the client
			if comma := strings.Index(ip, ","); comma != -1 {
				ip = ip[:comma]
			}
		} else {
			ip = r.Header.Get("X-Real-IP")
		}

		if ip == "" {
			// Fallback to direct connection IP
			var err error
			ip, _, err = net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				ip = r.RemoteAddr
			}
		}

		newID, err := store.AddEntry(newText, ip)
		if err != nil {
			slog.Error("Error saving entry", "error", err)
			http.Error(w, "Failed to save entry", http.StatusInternalServerError)
			return
		}
		slog.Info("Added Entry", "ip", ip, "text", newText)

		http.Redirect(w, r, fmt.Sprintf("/%d", newID), http.StatusFound)
	})
	return mux
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}

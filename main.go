package main

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"site/storage"
)

// PageData is passed to the template
type PageData struct {
	Content      string
	Timestamp    string
	Editing      bool
	PrevLink     string
	NextLink     string
	EditLink     string
	Entries      []storage.Entry
	CurrentIndex int
	TotalCount   int
}

const (
	defaultDBPath = "data/history.db"

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
		log.Printf("Warning: Invalid timezone '%s', defaulting to UTC. Error: %v", tz, err)
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

	tmpl := template.Must(template.ParseFiles("index.html"))

	mux := newServer(store, tmpl, loc)

	srv := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	// Start server in a goroutine
	go func() {
		log.Println("Server started at http://localhost:8080")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("listen: %s\n", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server with a timeout of 5 seconds.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("server forced to shutdown: %w", err)
	}

	log.Println("Server exiting")
	return nil
}

func newServer(store *storage.Store, tmpl *template.Template, loc *time.Location) http.Handler {
	mux := http.NewServeMux()

	// Redirect root to the newest entry
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		latest, err := store.GetLatestID()
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			log.Printf("Could not get latest ID: %v", err)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/%d", latest), http.StatusFound)
	})

	// Show form for a new entry
	mux.HandleFunc("GET /new", func(w http.ResponseWriter, r *http.Request) {
		latest, err := store.GetLatestID()
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			log.Printf("Could not get latest ID: %v", err)
			return
		}
		data := PageData{
			Editing:      true,
			CurrentIndex: latest + 1,
			TotalCount:   latest,
		}
		tmpl.Execute(w, data)
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
			log.Printf("Could not get latest ID: %v", err)
			return
		}
		if id < 1 || id > latest {
			http.NotFound(w, r)
			return
		}

		entry, err := store.GetEntry(id)
		if err != nil {
			http.Error(w, "Error retrieving entry", http.StatusInternalServerError)
			return
		}

		data := PageData{
			Content:      entry.Content,
			Timestamp:    entry.CreatedAt.In(loc).Format("Jan 02, 2006 15:04:05 MST"),
			CurrentIndex: id,
			TotalCount:   latest,
			EditLink:     "/new",
		}

		if id > 1 {
			data.PrevLink = fmt.Sprintf("/%d", id-1)
		}
		if id < latest {
			data.NextLink = fmt.Sprintf("/%d", id+1)
		}

		tmpl.Execute(w, data)
	})

	mux.HandleFunc("GET /list", func(w http.ResponseWriter, r *http.Request) {
		entries, err := store.GetAllEntries()
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			log.Printf("Could not get latest ID: %v", err)
			return
		}

		data := PageData{
			Entries: entries,
		}

		tmpl.Execute(w, data)
	})

	// Save a new entry
	mux.HandleFunc("POST /save", func(w http.ResponseWriter, r *http.Request) {
		newText := strings.TrimSpace(r.FormValue("newText"))

		if newText == "" {
			http.Error(w, "Content cannot be empty", http.StatusBadRequest)
			return
		}
		if len(newText) > maxLen {
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
			log.Printf("Error saving entry: %v", err)
			http.Error(w, "Failed to save entry", http.StatusInternalServerError)
			return
		}

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

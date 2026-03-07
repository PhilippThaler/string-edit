package main

import (
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
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
	CurrentIndex int
	TotalCount   int
}

const (
	defaultDBPath = "data/history.db"

	maxLen                  = 280
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

	log.Println("Server started at http://localhost:8080")
	return http.ListenAndServe(":8080", mux)
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

	// View or Edit a specific entry
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

		isEditing := r.URL.Query().Get("edit") == "true"

		data := PageData{
			Content:      entry.Content,
			Timestamp:    entry.CreatedAt.In(loc).Format("Jan 02, 2006 15:04:05 MST"),
			Editing:      isEditing,
			CurrentIndex: id,
			TotalCount:   latest,
			EditLink:     fmt.Sprintf("/%d?edit=true", id),
		}

		if id > 1 {
			data.PrevLink = fmt.Sprintf("/%d", id-1)
		}
		if id < latest {
			data.NextLink = fmt.Sprintf("/%d", id+1)
		}

		tmpl.Execute(w, data)
	})

	// Save a new entry
	mux.HandleFunc("POST /save", func(w http.ResponseWriter, r *http.Request) {
		newText := r.FormValue("newText")

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

package main

import (
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"site/storage"
	"strconv"
	"time"
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

const maxLen = 280

func main() {
	// Use a consistent path for the database
	const dbPath = "data/history.db"

	store, err := storage.NewStore(dbPath)
	if err != nil {
		log.Fatal("Failed to initialize database:", err)
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

	// Ensure we have at least one entry so the site isn't broken on fresh start
	if id, _ := store.GetLatestID(); id == 0 {
		store.AddEntry("Hello, World!", "system")
	}

	tmpl := template.Must(template.ParseFiles("index.html"))
	mux := http.NewServeMux()

	// Redirect root to the newest entry
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		latest, _ := store.GetLatestID()
		http.Redirect(w, r, fmt.Sprintf("/entry/%d", latest), http.StatusFound)
	})

	// View or Edit a specific entry
	mux.HandleFunc("GET /entry/{id}", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		latest, _ := store.GetLatestID()
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
			Timestamp:    entry.CreatedAt.In(loc).Format("Jan 02, 2006 15:04:05 UTC"),
			Editing:      isEditing,
			CurrentIndex: id,
			TotalCount:   latest,
			EditLink:     fmt.Sprintf("/entry/%d?edit=true", id),
		}

		if id > 1 {
			data.PrevLink = fmt.Sprintf("/entry/%d", id-1)
		}
		if id < latest {
			data.NextLink = fmt.Sprintf("/entry/%d", id+1)
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

		// Get IP address
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		if ip == "" {
			ip = r.RemoteAddr
		}

		newID, err := store.AddEntry(newText, ip)
		if err != nil {
			log.Printf("Error saving entry: %v", err)
			http.Error(w, "Failed to save entry", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, fmt.Sprintf("/entry/%d", newID), http.StatusFound)
	})

	log.Println("Server started at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

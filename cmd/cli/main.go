package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"volume_run/internal/garmin"
	"volume_run/internal/handlers"
	"volume_run/internal/store"
)

func main() {
	addr   := flag.String("addr", ":8080", "listen address")
	dbPath := flag.String("db", "./db/garmin.db", "SQLite database path")
	flag.Parse()

	if err := os.MkdirAll(filepath.Dir(*dbPath), 0755); err != nil {
		log.Fatalf("create db dir: %v", err)
	}

	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	gc := garmin.NewClient()

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir("static")))
	mux.HandleFunc("/api/status", handlers.Status(gc))
	mux.HandleFunc("/api/key", handlers.SetKey(gc))
	mux.HandleFunc("/api/weeks", handlers.Weeks(st))
	mux.HandleFunc("/api/sync", handlers.Sync(st, gc))
	mux.HandleFunc("/api/activities", handlers.Activities(st))

	log.Printf("listening on %s", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}

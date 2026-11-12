package main

import (
	"flag"
	"log"
	"net/http"

	"run_vol/internal/garmin"
	"run_vol/internal/handlers"
	"run_vol/internal/store"
)

func main() {
	addr      := flag.String("addr", ":8080", "listen address")
	dbPath    := flag.String("db", "./garmin.db", "SQLite database path")
	tokenFile := flag.String("token-file", "./garmin_token.json", "OAuth token file path")
	flag.Parse()

	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	gc, err := garmin.NewClient(*tokenFile)
	if err != nil {
		log.Fatalf("garmin client: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir("static")))
	mux.HandleFunc("/api/weeks", handlers.Weeks(st))
	mux.HandleFunc("/api/sync", handlers.Sync(st, gc))

	log.Printf("listening on %s", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}

package main

import (
	"clearcare/billreview"
	"clearcare/caresearch"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	dataDir := os.Getenv("CLEARCARE_DATA_DIR")
	if dataDir == "" {
		dataDir = "../data"
	}
	samplePath := filepath.Join(dataDir, "..", "sample", "bill_with_problems.json")
	if os.Getenv("CLEARCARE_DEMO") == "1" {
		dataDir = filepath.Join(dataDir, "demo")
	}
	care := caresearch.Load(dataDir)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /sample", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, samplePath)
	})
	mux.HandleFunc("POST /bills/analyze", billreview.HandleAnalyze)
	mux.HandleFunc("GET /providers", care.Providers)
	mux.HandleFunc("GET /prices/compare", care.Prices)
	mux.HandleFunc("GET /data/status", care.Status)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if origin := r.Header.Get("Origin"); origin != "" && origin != "http://127.0.0.1:4173" && origin != "http://localhost:4173" && origin != "http://127.0.0.1:8000" && origin != "http://localhost:8000" {
			http.Error(w, "Origin not allowed", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 11<<20)
		mux.ServeHTTP(w, r)
	})
	log.Println("ClearCare Go API listening on 127.0.0.1:8000")
	log.Fatal(http.ListenAndServe("127.0.0.1:8000", handler))
}
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

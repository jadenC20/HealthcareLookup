package main

import (
	"clearcare/account"
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
	dbPath := os.Getenv("CLEARCARE_DB")
	if dbPath == "" {
		dbPath = "../clearcare.db"
	}
	accounts, err := account.Open(dbPath)
	if err != nil {
		log.Fatalf("open account database: %v", err)
	}
	defer accounts.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /sample", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, samplePath)
	})
	mux.HandleFunc("POST /bills/analyze", billreview.HandleAnalyze)
	mux.HandleFunc("GET /providers", care.Providers)
	mux.HandleFunc("GET /prices/compare", care.Prices)
	mux.HandleFunc("GET /data/status", care.Status)
	mux.HandleFunc("POST /data/reload", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"detail":"Reload data by restarting the local backend."}`, http.StatusForbidden)
	})
	mux.HandleFunc("POST /auth/register", accounts.Register)
	mux.HandleFunc("POST /auth/login", accounts.Login)
	mux.HandleFunc("GET /auth/me", accounts.Me)
	mux.HandleFunc("POST /auth/logout", accounts.Logout)
	mux.HandleFunc("PATCH /profile", accounts.Profile)
	mux.HandleFunc("GET /profile/insurance-card", accounts.CardStatus)
	mux.HandleFunc("GET /profile/insurance-card/file", accounts.CardFile)
	mux.HandleFunc("POST /profile/insurance-card", accounts.UploadCard)
	mux.HandleFunc("DELETE /profile/insurance-card", accounts.DeleteCard)
	mux.HandleFunc("GET /profile/insurance", accounts.InsuranceStatus)
	mux.HandleFunc("PUT /profile/insurance/details", accounts.ConfirmInsurance)
	mux.HandleFunc("POST /profile/insurance/retry", accounts.RetryInsurance)
	mux.HandleFunc("POST /profile/insurance/select", accounts.SelectInsurance)
	mux.HandleFunc("POST /profile/insurance/sbc", accounts.UploadSBC)
	mux.HandleFunc("GET /profile/insurance/sbc", accounts.SBCFile)
	mux.HandleFunc("DELETE /profile/insurance", accounts.DeleteInsurance)
	mux.HandleFunc("DELETE /account", accounts.DeleteAccount)
	mux.HandleFunc("POST /documents/extract", accounts.Extract)
	mux.HandleFunc("POST /reviews", accounts.Save)
	mux.HandleFunc("GET /reviews", accounts.List)
	mux.HandleFunc("GET /reviews/{id}", accounts.Get)
	mux.HandleFunc("DELETE /reviews/{id}", accounts.Delete)
	mux.HandleFunc("GET /reviews/{id}/pdf", accounts.SavedPDF)
	mux.HandleFunc("POST /reports/pdf", accounts.CurrentPDF)
	mux.HandleFunc("GET /account/pdf", accounts.AccountPDF)
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

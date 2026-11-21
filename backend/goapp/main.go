package main

import (
	"clearcare/account"
	"clearcare/billreview"
	"clearcare/caresearch"
	"clearcare/prescription"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	address := os.Getenv("CLEARCARE_ADDR")
	if address == "" {
		address = "127.0.0.1:8000"
	}
	host, _, addressErr := net.SplitHostPort(address)
	if addressErr != nil || (host != "127.0.0.1" && host != "localhost" && host != "::1") {
		log.Fatal("CLEARCARE_ADDR must be a loopback host:port")
	}
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
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"status": "ok", "features": map[string]bool{"supabase_auth": true, "insurance": true, "bill_review": true, "care_search": true, "prescription_comparison": true}, "configuration": map[string]bool{"supabase": os.Getenv("SUPABASE_URL") != "" && (os.Getenv("SUPABASE_PUBLISHABLE_KEY") != "" || os.Getenv("SUPABASE_ANON_KEY") != ""), "openai": os.Getenv("OPENAI_API_KEY") != ""}})
	})
	mux.HandleFunc("GET /sample", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, samplePath)
	})
	mux.HandleFunc("POST /bills/analyze", billreview.HandleAnalyze)
	mux.HandleFunc("POST /bills/ai-review", accounts.AIReview)
	mux.HandleFunc("POST /bill-review/extract", accounts.AsyncFeature("bill_extraction", func(w http.ResponseWriter, r *http.Request) { billreview.ExtractDocuments(r.Context(), w, r) }, false, true))
	mux.HandleFunc("POST /bill-review/analyze", accounts.AsyncFeature("bill_review", accounts.ReviewBill, true, false))
	mux.HandleFunc("GET /bill-review/observations", accounts.Observations)
	mux.HandleFunc("POST /care-search/recommend", accounts.AsyncFeature("care_search", accounts.Recommend(care), false, false))
	mux.HandleFunc("GET /care/recommendations", accounts.CareRecommendations(care))
	mux.HandleFunc("GET /prescription-comparison/drugs", accounts.Protected(false, prescription.Drugs))
	mux.HandleFunc("POST /prescription-comparison/compare", accounts.AsyncFeature("prescription_comparison", prescription.Compare, false, true))
	mux.HandleFunc("GET /jobs", accounts.Jobs)
	mux.HandleFunc("GET /jobs/{id}", accounts.Jobs)
	mux.HandleFunc("DELETE /jobs/{id}", accounts.Jobs)
	mux.HandleFunc("GET /profile/preferences", accounts.Preferences)
	mux.HandleFunc("PUT /profile/preferences", accounts.Preferences)
	mux.HandleFunc("GET /results", accounts.Runs)
	mux.HandleFunc("GET /results/{id}", accounts.Runs)
	mux.HandleFunc("DELETE /results/{id}", accounts.Runs)
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
		if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+address && origin != "http://127.0.0.1:4173" && origin != "http://localhost:4173" && origin != "http://127.0.0.1:4182" && origin != "http://localhost:4182" && origin != "http://127.0.0.1:8000" && origin != "http://localhost:8000" {
			http.Error(w, "Origin not allowed", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 21<<20)
		mux.ServeHTTP(w, r)
	})
	log.Println("ClearCare Go API listening on " + address)
	log.Fatal(http.ListenAndServe(address, handler))
}
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

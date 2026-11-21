package main

import (
	"clearcare/account"
	"clearcare/billreview"
	"clearcare/caresearch"
	"clearcare/insurance"
	"clearcare/prescription"
	"crypto/subtle"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	runtime, err := loadRuntimeConfig()
	if err != nil {
		log.Fatalf("load runtime configuration: %v", err)
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
	var accounts *account.Service
	if runtime.StoreMode == "postgres" {
		accounts, err = account.OpenPostgres(runtime.StoreDSN)
	} else {
		accounts, err = account.OpenLocalSQLite(runtime.StoreDSN)
	}
	if err != nil {
		log.Fatalf("open account database: %v", err)
	}
	defer accounts.Close()
	mux := http.NewServeMux()
	aiConfig := insurance.NewOpenAI()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		provider, model, configured := aiConfig.Provider, aiConfig.Model, aiConfig.Key != ""
		writeJSON(w, map[string]any{"status": "ok", "store": runtime.StoreMode, "ai": map[string]any{"provider": provider, "model": model, "configured": configured}, "features": map[string]bool{"supabase_auth": true, "insurance": true, "bill_review": true, "care_search": true, "prescription_comparison": true}, "configuration": map[string]bool{"supabase": os.Getenv("SUPABASE_URL") != "" && (os.Getenv("SUPABASE_PUBLISHABLE_KEY") != "" || os.Getenv("SUPABASE_ANON_KEY") != ""), "openai": provider == "openai" && configured, "ai": configured}})
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
	mux.HandleFunc("GET /care/search", accounts.SearchCare)
	mux.HandleFunc("GET /prescription-comparison/drugs", accounts.Protected(false, prescription.Drugs))
	mux.HandleFunc("POST /prescription-comparison/compare", accounts.AsyncFeature("prescription_comparison", prescription.Compare, false, true))
	mux.HandleFunc("POST /prescription-comparison/search", accounts.AsyncFeature("prescription_search", accounts.MedicineSearch(prescription.Search), false, true))
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
	mux.HandleFunc("GET /profile/insurance/context", accounts.InsuranceContext)
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
		if secret := os.Getenv("CLEARCARE_PROXY_SECRET"); secret != "" && r.URL.Path != "/health" {
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-ClearCare-Proxy-Secret")), []byte(secret)) != 1 {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
		}
		if origin := r.Header.Get("Origin"); !runtime.originAllowed(origin) {
			http.Error(w, "Origin not allowed", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 21<<20)
		mux.ServeHTTP(w, r)
	})
	log.Printf("ClearCare Go API listening on %s with %s storage", runtime.Address, runtime.StoreMode)
	log.Fatal(http.ListenAndServe(runtime.Address, handler))
}
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

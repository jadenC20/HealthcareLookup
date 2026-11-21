package account

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func supabaseTestService(t *testing.T) *Service {
	t.Helper()
	t.Setenv("OPENAI_API_KEY", "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/v1/user" || r.Header.Get("apikey") != "sb_publishable_test" {
			t.Error("verification must use the configured project and apikey")
			w.WriteHeader(401)
			return
		}
		id := "11111111-1111-4111-8111-111111111111"
		confirmed, role := "2026-11-21T00:00:00Z", "authenticated"
		switch r.Header.Get("Authorization") {
		case "Bearer first":
		case "Bearer second":
			id = "22222222-2222-4222-8222-222222222222"
		case "Bearer unconfirmed":
			confirmed = ""
		case "Bearer invalid-role":
			role = "service_role"
		case "Bearer unavailable":
			w.WriteHeader(503)
			return
		default:
			w.WriteHeader(401)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"id": id, "email": "same@example.test", "role": role, "email_confirmed_at": confirmed, "user_metadata": map[string]string{"display_name": "Test member", "id": "legacy-account-id"}})
	}))
	t.Cleanup(server.Close)
	s, err := Open(filepath.Join(t.TempDir(), "supabase.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	s.supabase = &supabaseVerifier{origin: server.URL, key: "sb_publishable_test", client: server.Client()}
	return s
}

func TestSupabaseProfileAndReviewsUseOneVerifiedIdentity(t *testing.T) {
	s := supabaseTestService(t)
	// A same-email legacy user must never be silently linked to a new identity.
	_, err := s.db.Exec(`INSERT INTO users(id,email,name,password,plan) VALUES('legacy','same@example.test','Legacy','legacy-only','Private legacy plan')`)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /auth/me", s.Me)
	mux.HandleFunc("PATCH /profile", s.Profile)
	mux.HandleFunc("POST /reviews", s.Save)
	mux.HandleFunc("GET /reviews", s.List)
	mux.HandleFunc("GET /reviews/{id}", s.Get)
	mux.HandleFunc("GET /account/pdf", s.AccountPDF)
	mux.HandleFunc("GET /profile/insurance", s.InsuranceStatus)
	mux.HandleFunc("POST /auth/logout", s.Logout)
	call := func(method, path, token, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	w := call("GET", "/auth/me", "first", "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var me struct {
		User User   `json:"user"`
		CSRF string `json:"csrf"`
	}
	json.Unmarshal(w.Body.Bytes(), &me)
	if me.User.ID != "supabase:11111111-1111-4111-8111-111111111111" || me.User.Plan != "" || me.User.Email != "same@example.test" || me.CSRF != "" {
		t.Fatalf("wrong identity: %+v", me)
	}
	w = call("PATCH", "/profile", "first", `{"name":"Updated","city":"Philadelphia","plan":"My insurance plan"}`)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	w = call("GET", "/auth/me", "first", "")
	if !strings.Contains(w.Body.String(), "My insurance plan") {
		t.Fatal("plan did not persist")
	}
	w = call("GET", "/auth/me", "second", "")
	if strings.Contains(w.Body.String(), "My insurance plan") {
		t.Fatal("cross-user profile leak")
	}
	w = call("POST", "/reviews", "first", `{"bill":{"provider_name":"Test clinic","lines":[{"line_id":"L1","quantity":1,"charge":75,"description":"Visit"}]}}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var saved review
	json.Unmarshal(w.Body.Bytes(), &saved)
	if w = call("GET", "/reviews/"+saved.ID, "second", ""); w.Code != 404 {
		t.Fatal("cross-user review access", w.Code)
	}
	if w = call("GET", "/reviews", "first", ""); !strings.Contains(w.Body.String(), saved.ID) {
		t.Fatal("saved review not listed")
	}
	if w = call("GET", "/account/pdf", "first", ""); w.Code != 200 || !strings.HasPrefix(w.Body.String(), "%PDF-") {
		t.Fatal("PDF export failed", w.Code)
	}
	if w = call("GET", "/profile/insurance", "first", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "not_started") {
		t.Fatal("insurance status failed", w.Code)
	}
	if w = call("POST", "/auth/logout", "first", ""); w.Code != 200 {
		t.Fatal("bearer logout panicked or failed")
	}
}

func TestSupabaseRejectsInvalidIdentityAndNeverFallsBackToCookie(t *testing.T) {
	s := supabaseTestService(t)
	// A valid legacy cookie cannot rescue an invalid bearer token.
	s.db.Exec(`INSERT INTO users(id,email,name,password) VALUES('legacy','legacy@example.test','Legacy','unused')`)
	w := httptest.NewRecorder()
	_, err := s.issue(w, "legacy")
	if err != nil {
		t.Fatal(err)
	}
	cookie := w.Result().Cookies()[0]
	for _, item := range []struct {
		token  string
		status int
	}{{"invalid", 401}, {"unconfirmed", 401}, {"invalid-role", 401}, {"unavailable", 503}} {
		t.Run(item.token, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/auth/me", nil)
			r.AddCookie(cookie)
			r.Header.Set("Authorization", "Bearer "+item.token)
			w := httptest.NewRecorder()
			s.Me(w, r)
			if w.Code != item.status {
				t.Fatalf("got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestSupabaseHealthcareDeletionIsScopedAndLeavesLoginIndependent(t *testing.T) {
	s := supabaseTestService(t)
	for _, token := range []string{"first", "second"} {
		r := httptest.NewRequest("PATCH", "/profile", strings.NewReader(`{"name":"Member","city":"","plan":"Saved plan"}`))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		s.Profile(w, r)
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
	}
	r := httptest.NewRequest("DELETE", "/account", nil)
	r.Header.Set("Authorization", "Bearer first")
	w := httptest.NewRecorder()
	s.DeleteAccount(w, r)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	for _, token := range []string{"first", "second"} {
		r := httptest.NewRequest("GET", "/auth/me", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		s.Me(w, r)
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		hasPlan := strings.Contains(w.Body.String(), "Saved plan")
		if hasPlan != (token == "second") {
			t.Fatal("deletion scope incorrect", token)
		}
	}
}

func TestSupabaseVerificationDoesNotFollowRedirects(t *testing.T) {
	reached := false
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))
	defer destination.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, http.StatusFound) }))
	defer origin.Close()
	t.Setenv("SUPABASE_URL", origin.URL)
	t.Setenv("SUPABASE_PUBLISHABLE_KEY", "sb_publishable_test")
	s := supabaseTestService(t)
	s.supabase = newSupabaseVerifier()
	r := httptest.NewRequest("GET", "/auth/me", nil)
	r.Header.Set("Authorization", "Bearer first")
	w := httptest.NewRecorder()
	s.Me(w, r)
	if reached || w.Code != 503 {
		t.Fatal("verification followed a redirect")
	}
}

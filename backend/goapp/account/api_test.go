package account

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestAccountReviewLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth/register", s.Register)
	mux.HandleFunc("POST /auth/login", s.Login)
	mux.HandleFunc("POST /auth/logout", s.Logout)
	mux.HandleFunc("POST /reviews", s.Save)
	mux.HandleFunc("GET /reviews/{id}", s.Get)
	mux.HandleFunc("GET /reviews/{id}/pdf", s.SavedPDF)
	call := func(method, path, body, csrf string, c *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		if c != nil {
			r.AddCookie(c)
		}
		r.Header.Set("X-CSRF-Token", csrf)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	register := func(email string) (*http.Cookie, string) {
		w := call("POST", "/auth/register", `{"name":"Test","email":"`+email+`","password":"test-password-12345"}`, "", nil)
		if w.Code != 200 {
			t.Fatalf("register: %d %s", w.Code, w.Body.String())
		}
		var result struct {
			CSRF string `json:"csrf"`
		}
		json.Unmarshal(w.Body.Bytes(), &result)
		return w.Result().Cookies()[0], result.CSRF
	}
	cookie, csrf := register("first@example.org")
	body := `{"bill":{"provider_name":"Clinic","lines":[{"line_id":"L1","description":"Office visit","quantity":1,"charge":100,"page":2}]}}`
	if w := call("POST", "/reviews", body, "", cookie); w.Code != 403 {
		t.Fatalf("missing csrf: %d", w.Code)
	}
	w := call("POST", "/reviews", body, csrf, cookie)
	if w.Code != 200 {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
	var saved struct {
		ID string `json:"id"`
	}
	json.Unmarshal(w.Body.Bytes(), &saved)
	if w = call("GET", "/reviews/"+saved.ID+"/pdf", "", "", cookie); w.Code != 200 || !bytes.HasPrefix(w.Body.Bytes(), []byte("%PDF-")) {
		t.Fatalf("pdf: %d", w.Code)
	}
	other, _ := register("second@example.org")
	if w = call("GET", "/reviews/"+saved.ID, "", "", other); w.Code != 404 {
		t.Fatalf("cross-user access: %d", w.Code)
	}
	if w = call("GET", "/reviews/"+saved.ID, "", "", cookie); !bytes.Contains(w.Body.Bytes(), []byte(`"page":2`)) {
		t.Fatalf("page lost: %s", w.Body.String())
	}
	if w = call("POST", "/auth/login", `{"email":"first@example.org","password":"test-password-12345"}`, "", nil); w.Code != 200 {
		t.Fatalf("login: %d %s", w.Code, w.Body.String())
	}
}

func TestInsuranceCardLifecycle(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	s, err := Open(filepath.Join(t.TempDir(), "cards.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	register := func(email string) (*http.Cookie, string, string) {
		r := httptest.NewRequest("POST", "/auth/register", strings.NewReader(`{"name":"Test","email":"`+email+`","password":"test-password-12345"}`))
		w := httptest.NewRecorder()
		s.Register(w, r)
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		var v struct {
			CSRF string `json:"csrf"`
			User User   `json:"user"`
		}
		json.Unmarshal(w.Body.Bytes(), &v)
		return w.Result().Cookies()[0], v.CSRF, v.User.ID
	}
	cookie, csrf, uid := register("a@example.org")
	other, _, _ := register("b@example.org")
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	part, _ := mw.CreateFormFile("file", "card.pdf")
	part.Write([]byte("%PDF-1.4\nexample"))
	mw.Close()
	r := httptest.NewRequest("POST", "/profile/insurance-card", &b)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.Header.Set("X-CSRF-Token", csrf)
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	s.UploadCard(w, r)
	if w.Code != 200 {
		t.Fatalf("upload: %d %s", w.Code, w.Body.String())
	}
	r = httptest.NewRequest("GET", "/profile/insurance-card", nil)
	r.AddCookie(other)
	w = httptest.NewRecorder()
	s.CardStatus(w, r)
	if !strings.Contains(w.Body.String(), `"has_card":false`) {
		t.Fatalf("card leaked: %s", w.Body.String())
	}
	r = httptest.NewRequest("DELETE", "/account", nil)
	r.Header.Set("X-CSRF-Token", csrf)
	r.AddCookie(cookie)
	w = httptest.NewRecorder()
	s.DeleteAccount(w, r)
	if w.Code != 200 {
		t.Fatalf("delete: %d", w.Code)
	}
	var count int
	s.db.QueryRow(`SELECT count(*) FROM insurance_cards WHERE user_id=?`, uid).Scan(&count)
	if count != 0 {
		t.Fatal("card not deleted with account")
	}
}

package account

import (
	"bytes"
	"clearcare/insurance"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

type insuranceAI struct{}

func (insuranceAI) Extract(context.Context, insurance.Document) (insurance.Extraction, error) {
	return insurance.Extraction{Details: profileDetails()}, nil
}
func (insuranceAI) Discover(context.Context, insurance.Details) ([]insurance.Candidate, error) {
	return []insurance.Candidate{}, nil
}
func (insuranceAI) Interpret(context.Context, insurance.Document) (insurance.Interpretation, error) {
	return insurance.Interpretation{IsSBC: true, Details: profileDetails(), Benefits: []insurance.Benefit{{Name: "deductible", Value: "$500 individual in-network", Quote: "Individual deductible: $500", Page: 1}}}, nil
}

type insuranceCatalog struct{}

func (insuranceCatalog) Find(context.Context, insurance.Details) ([]insurance.Candidate, error) {
	return []insurance.Candidate{{Details: profileDetails(), URL: "https://insurer.example/sbc.pdf", Source: "cms_catalog"}}, nil
}

type insuranceFetcher struct{}

func (insuranceFetcher) Fetch(context.Context, string) (insurance.Document, error) {
	return insurance.NewDocument([]byte("%PDF-1.4\nfixture"), "application/pdf", "https://insurer.example/sbc.pdf"), nil
}
func profileDetails() insurance.Details {
	return insurance.Details{Insurer: "Example", PlanName: "Example Gold", PlanID: "12345PA1234567-01", Year: 2026, State: "PA"}
}
func insuranceTestService(t *testing.T) (*Service, *http.Cookie, string, string) {
	t.Helper()
	t.Setenv("OPENAI_API_KEY", "")
	s, err := Open(filepath.Join(t.TempDir(), "insurance.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	s.insurance = &insurance.Engine{AI: insuranceAI{}, Catalog: insuranceCatalog{}, Fetcher: insuranceFetcher{}}
	w := httptest.NewRecorder()
	s.Register(w, httptest.NewRequest("POST", "/auth/register", strings.NewReader(`{"name":"Test","email":"user@example.org","password":"test-password-12345"}`)))
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var result struct {
		User User   `json:"user"`
		CSRF string `json:"csrf"`
	}
	json.Unmarshal(w.Body.Bytes(), &result)
	return s, w.Result().Cookies()[0], result.CSRF, result.User.ID
}
func insuranceCall(t *testing.T, handler http.HandlerFunc, cookie *http.Cookie, csrf string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	r := httptest.NewRequest("POST", "/", bytes.NewReader(b))
	r.AddCookie(cookie)
	r.Header.Set("X-CSRF-Token", csrf)
	w := httptest.NewRecorder()
	handler(w, r)
	return w
}
func readState(t *testing.T, w *httptest.ResponseRecorder) insuranceState {
	t.Helper()
	if w.Code != 200 {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}
	var state insuranceState
	if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	return state
}
func TestConfirmedLookupPersistsAndDownloads(t *testing.T) {
	s, cookie, csrf, uid := insuranceTestService(t)
	w := insuranceCall(t, s.ConfirmInsurance, cookie, csrf, map[string]any{"revision": "", "details": profileDetails()})
	state := readState(t, w)
	if state.Result.Status != "ready_with_caution" || !state.Result.MayBeInaccurate || len(state.Result.Benefits) != 1 {
		t.Fatalf("unexpected: %+v", state)
	}
	saved, err := s.loadInsurance(uid)
	if err != nil || saved.Revision != state.Revision || saved.Result.Document.SHA256 == "" {
		t.Fatal("result not persisted")
	}
	w = insuranceCall(t, s.SBCFile, cookie, "", nil)
	if w.Code != 200 || !bytes.HasPrefix(w.Body.Bytes(), []byte("%PDF-")) {
		t.Fatal("document missing")
	}
	w = insuranceCall(t, s.ConfirmInsurance, cookie, csrf, map[string]any{"revision": "stale", "details": profileDetails()})
	if w.Code != 409 {
		t.Fatal("stale mutation accepted")
	}
	w = insuranceCall(t, s.ConfirmInsurance, cookie, "", map[string]any{"revision": state.Revision, "details": profileDetails()})
	if w.Code != 403 {
		t.Fatal("CSRF not enforced")
	}
	w = insuranceCall(t, s.SelectInsurance, cookie, csrf, map[string]any{"revision": state.Revision, "candidate_id": "https://127.0.0.1/"})
	if w.Code != 422 {
		t.Fatal("arbitrary URL accepted")
	}
}
func uploadRequest(cookie *http.Cookie, csrf, revision string) *http.Request {
	var b bytes.Buffer
	m := multipart.NewWriter(&b)
	if revision != "" {
		m.WriteField("revision", revision)
	}
	p, _ := m.CreateFormFile("file", "document.pdf")
	p.Write([]byte("%PDF-1.4\nsynthetic"))
	m.Close()
	r := httptest.NewRequest("POST", "/", &b)
	r.Header.Set("Content-Type", m.FormDataContentType())
	r.Header.Set("X-CSRF-Token", csrf)
	r.AddCookie(cookie)
	return r
}
func TestCardUploadAutomaticallyStartsLookup(t *testing.T) {
	s, cookie, csrf, _ := insuranceTestService(t)
	w := httptest.NewRecorder()
	s.UploadCard(w, uploadRequest(cookie, csrf, ""))
	state := readState(t, w)
	if state.Result.Status != "needs_confirmation" || len(state.Result.Candidates) != 1 {
		t.Fatalf("lookup not started: %+v", state)
	}
	w = insuranceCall(t, s.SelectInsurance, cookie, csrf, map[string]any{"revision": state.Revision, "candidate_id": state.Result.Candidates[0].ID})
	state = readState(t, w)
	if state.Result.Status != "ready_with_caution" {
		t.Fatalf("selection failed: %+v", state)
	}
}
func TestMissingKeyPersistsCautionAndManualFallback(t *testing.T) {
	s, cookie, csrf, uid := insuranceTestService(t)
	s.insurance.AI = insurance.NewOpenAI()
	w := httptest.NewRecorder()
	s.UploadSBC(w, uploadRequest(cookie, csrf, ""))
	state := readState(t, w)
	if state.Result.Status != "partial" || !strings.Contains(w.Body.String(), "openai_not_configured") || state.Result.Document == nil {
		t.Fatalf("missing caution: %s", w.Body.String())
	}
	saved, err := s.loadInsurance(uid)
	if err != nil || !saved.Result.MayBeInaccurate {
		t.Fatal("partial state missing")
	}
	s.insurance.AI = insuranceAI{}
	w = insuranceCall(t, s.RetryInsurance, cookie, csrf, map[string]any{"revision": state.Revision})
	retried := readState(t, w)
	if len(retried.Result.Benefits) != 1 || retried.Result.Document.SHA256 != state.Result.Document.SHA256 {
		t.Fatal("manual SBC retry lost its document or failed to interpret")
	}
}
func TestOldRunCannotResurrectDeletedInsurance(t *testing.T) {
	s, cookie, csrf, uid := insuranceTestService(t)
	state := readState(t, insuranceCall(t, s.ConfirmInsurance, cookie, csrf, map[string]any{"revision": "", "details": profileDetails()}))
	w := insuranceCall(t, s.DeleteInsurance, cookie, csrf, nil)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	w = httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", nil)
	s.runInsurance(w, r, uid, state.Revision, func(context.Context) insurance.Result { return state.Result })
	if w.Code != 409 {
		t.Fatal("stale result accepted")
	}
	if _, err := s.loadInsurance(uid); err == nil {
		t.Fatal("deleted state resurrected")
	}
}
func TestInsuranceResultIsPrivate(t *testing.T) {
	s, cookie, csrf, _ := insuranceTestService(t)
	readState(t, insuranceCall(t, s.ConfirmInsurance, cookie, csrf, map[string]any{"revision": "", "details": profileDetails()}))
	w := httptest.NewRecorder()
	s.Register(w, httptest.NewRequest("POST", "/auth/register", strings.NewReader(`{"name":"Other","email":"other@example.org","password":"test-password-12345"}`)))
	other := w.Result().Cookies()[0]
	w = insuranceCall(t, s.InsuranceStatus, other, "", nil)
	state := readState(t, w)
	if state.Result.Status != "not_started" || len(state.Result.Benefits) != 0 {
		t.Fatal("cross-user result leak")
	}
	w = insuranceCall(t, s.SBCFile, other, "", nil)
	if w.Code != 404 {
		t.Fatal("cross-user document leak")
	}
}

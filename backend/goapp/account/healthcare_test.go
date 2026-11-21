package account

import (
	"clearcare/caresearch"
	"clearcare/insurance"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeHealthcareAI struct {
	context insurance.HealthContext
	calls   int
}

func (f *fakeHealthcareAI) ExtractBill(context.Context, insurance.Document) (insurance.BillExtraction, error) {
	return insurance.BillExtraction{}, nil
}
func (f *fakeHealthcareAI) ReviewBill(_ context.Context, _ any, _ map[string]any, c insurance.HealthContext) (insurance.BillAdvice, error) {
	f.context = c
	f.calls++
	return insurance.BillAdvice{Summary: "AI explained the visit", Items: []insurance.BillAdviceItem{{Lines: []string{"L1"}, Explanation: "Confirm the itemized service", Question: "What is included?"}}, Inquiry: "Please clarify this bill.", Limitations: []string{"Coverage unverified"}}, nil
}
func (f *fakeHealthcareAI) ExplainCare(_ context.Context, _ []map[string]any, c insurance.HealthContext) (insurance.CareAdvice, error) {
	f.context = c
	f.calls++
	return insurance.CareAdvice{Summary: "Discuss the plan requirements", Recommendations: []insurance.CareAdviceItem{{Index: 0, Explanation: "Confirm network status"}}, Limitations: []string{"Coverage unverified"}}, nil
}
func TestAIReviewAndSavedPDFUseTheVerifiedAccountPlan(t *testing.T) {
	s := supabaseTestService(t)
	ai := &fakeHealthcareAI{}
	s.healthAI = ai
	mux := http.NewServeMux()
	mux.HandleFunc("PATCH /profile", s.Profile)
	mux.HandleFunc("POST /bills/ai-review", s.AIReview)
	mux.HandleFunc("POST /reviews", s.Save)
	mux.HandleFunc("POST /reports/pdf", s.CurrentPDF)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer first")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	if w := call("PATCH", "/profile", `{"name":"Member","city":"Philadelphia","plan":"Account plan"}`); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	body := `{"bill":{"provider_name":"Clinic","lines":[{"line_id":"L1","description":"Visit","quantity":1,"charge":75}]},"ai_consent":true,"save_ai":true,"plan_label":"FORGED_PLAN"}`
	if w := call("POST", "/bills/ai-review", body); w.Code != 200 || !strings.Contains(w.Body.String(), "AI explained the visit") {
		t.Fatal(w.Code, w.Body.String())
	}
	if ai.context.PlanLabel != "Account plan" {
		t.Fatal("untrusted or missing plan context", ai.context)
	}
	w := call("POST", "/reviews", body)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "AI explained the visit") {
		t.Fatal("saved review discarded AI output", w.Body.String())
	}
	if w := call("POST", "/reports/pdf", body); w.Code != 200 || !strings.HasPrefix(w.Body.String(), "%PDF-") {
		t.Fatal("AI report PDF failed", w.Code)
	}
}
func TestCareAIUsesStoredPlanAndOnlySourcedListings(t *testing.T) {
	s := supabaseTestService(t)
	ai := &fakeHealthcareAI{}
	s.healthAI = ai
	r := httptest.NewRequest("PATCH", "/profile", strings.NewReader(`{"name":"Member","city":"Philadelphia","plan":"Stored plan"}`))
	r.Header.Set("Authorization", "Bearer first")
	s.Profile(httptest.NewRecorder(), r)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "providers.csv"), []byte("provider_id,name,city,specialty,source_url,retrieved_date\nP1,Clinic,Philadelphia,Imaging,https://clinic.example,2026-11-21\n"), 0600)
	os.WriteFile(filepath.Join(dir, "service_prices.csv"), []byte("provider_id,procedure_code,price,price_type,includes,insurance_status,source_url,retrieved_date\nP1,73721,75,discounted_cash,facility,unknown,https://clinic.example/prices,2026-11-21\n"), 0600)
	handler := s.CareRecommendations(caresearch.Load(dir))
	for _, code := range []string{"73721", "missing"} {
		r := httptest.NewRequest("GET", "/care/recommendations?procedure_code="+code+"&ai_consent=true", nil)
		r.Header.Set("Authorization", "Bearer first")
		w := httptest.NewRecorder()
		handler(w, r)
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		var data map[string]any
		json.Unmarshal(w.Body.Bytes(), &data)
		if data["coverage_status"] != "unverified" {
			t.Fatal("coverage was invented")
		}
		if code == "missing" && !strings.Contains(w.Body.String(), "no_data") {
			t.Fatal("empty data must not invent listings")
		}
	}
	if ai.calls != 1 || ai.context.PlanLabel != "Stored plan" {
		t.Fatal("plan not used or AI called on empty catalog", ai)
	}
}

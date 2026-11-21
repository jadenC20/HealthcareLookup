package caresearch

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestDemoPriceComparison(t *testing.T) {
	s := Load("../../data/demo")
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/prices/compare?procedure_code=73721&city=Philadelphia", nil)
	s.Prices(w, r)
	var result struct {
		Results  []map[string]any `json:"results"`
		Warnings []string         `json:"warnings"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Results) != 3 {
		t.Fatalf("got %d results", len(result.Results))
	}
	if len(result.Warnings) == 0 {
		t.Fatal("expected warnings")
	}
}

func TestPriceComparisonAcceptsPlainServiceAndProviderQueries(t *testing.T) {
	s := &Service{
		providers: []row{{"provider_id": "p1", "name": "North Imaging", "specialty": "Radiology", "city": "Philadelphia"}, {"provider_id": "p2", "name": "Care Clinic", "specialty": "Primary Care", "city": "Boston"}},
		prices: []row{
			{"provider_id": "p1", "procedure_code": "73721", "description": "MRI knee without contrast", "price": "500", "price_type": "discounted_cash", "includes": "Facility"},
			{"provider_id": "p1", "procedure_code": "", "description": "Chest X-ray", "price": "90", "price_type": "discounted_cash", "includes": "Facility"},
			{"provider_id": "p2", "procedure_code": "99213", "description": "Office visit", "price": "120", "price_type": "gross_charge", "includes": "Office"},
		},
	}
	for _, test := range []struct {
		query url.Values
		want  int
	}{
		{url.Values{"q": {"MRI knee"}}, 1},
		{url.Values{"q": {"chest X-RAY"}}, 1},
		{url.Values{"q": {"NORTH radiology"}}, 2},
		{url.Values{"q": {"MRI"}, "procedure_code": {"old-hidden-filter"}}, 1},
		{url.Values{"q": {"MRI"}, "city": {"Boston"}}, 0},
		{url.Values{"q": {"office"}, "price_type": {"discounted_cash"}}, 0},
		{url.Values{}, 3},
		{url.Values{"city": {"philadelphia"}, "price_type": {"discounted_cash"}}, 2},
	} {
		got, _ := s.Compare(test.query)
		if len(got) != test.want {
			t.Errorf("query %v returned %d results, want %d", test.query, len(got), test.want)
		}
	}
	got, _ := s.Compare(url.Values{"q": {"chest"}})
	if got[0]["procedure_code"] != "" {
		t.Fatal("code-less service received an invented code")
	}
}

func TestPriceEndpointReturnsEmptyDataWithoutRequiringCode(t *testing.T) {
	s := &Service{providers: []row{}, prices: []row{}}
	for _, endpoint := range []string{"/prices/compare", "/prices/compare?q=MRI", "/prices/compare?procedure_code=73721"} {
		w := httptest.NewRecorder()
		s.Prices(w, httptest.NewRequest("GET", endpoint, nil))
		var body struct {
			Results []row `json:"results"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &body) != nil || body.Results == nil || len(body.Results) != 0 {
			t.Fatalf("empty-data endpoint %q failed: %d %s", endpoint, w.Code, w.Body.String())
		}
	}
}

func TestPriceBrowsingIsBoundedAndSorted(t *testing.T) {
	s := &Service{providers: []row{{"provider_id": "p1", "name": "Clinic"}}}
	for i := 110; i > 0; i-- {
		s.prices = append(s.prices, row{"provider_id": "p1", "description": "Office visit", "price": fmt.Sprint(i), "price_type": "discounted_cash"})
	}
	got, warnings := s.Compare(url.Values{})
	if len(got) != 100 || got[0]["price"] != float64(1) || got[99]["price"] != float64(100) || len(warnings) == 0 {
		t.Fatalf("unbounded or unsorted browsing: count=%d warnings=%v", len(got), warnings)
	}
}

func TestPriceLoadingAcceptsSourcedDescriptionWithoutCode(t *testing.T) {
	dir := t.TempDir()
	providers := "provider_id,name,specialty,city,source_url,retrieved_date\np1,Clinic,Primary Care,Boston,https://clinic.example,2026-11-20\n"
	prices := "provider_id,procedure_code,description,price,price_type,insurance_status,source_url,retrieved_date\np1,,Office visit,120,discounted_cash,unknown,https://clinic.example/prices,2026-11-20\np1,,,90,discounted_cash,unknown,https://clinic.example/prices,2026-11-20\np1,,Unsourced service,80,discounted_cash,unknown,,2026-11-20\np1,,Invalid amount,NaN,discounted_cash,unknown,https://clinic.example/prices,2026-11-20\n"
	if err := os.WriteFile(filepath.Join(dir, "providers.csv"), []byte(providers), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "service_prices.csv"), []byte(prices), 0600); err != nil {
		t.Fatal(err)
	}
	s := Load(dir)
	got, _ := s.Compare(url.Values{"q": {"office visit"}})
	if len(got) != 1 || len(s.prices) != 1 || got[0]["procedure_code"] != "" || got[0]["source_url"] != "https://clinic.example/prices" {
		t.Fatalf("code-less source handling changed quality validation: %#v", got)
	}
}

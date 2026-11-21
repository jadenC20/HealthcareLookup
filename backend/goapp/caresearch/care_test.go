package caresearch

import (
	"encoding/json"
	"net/http/httptest"
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

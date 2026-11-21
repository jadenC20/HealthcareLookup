package account

import (
	"clearcare/insurance"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type careRegistryTransport func(*http.Request) (*http.Response, error)

func (f careRegistryTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type directoryRankingAI struct {
	fakeHealthcareAI
	advice insurance.CareAdvice
	err    error
	query  string
}

func (f *directoryRankingAI) RankCare(_ context.Context, query string, _ []map[string]any, plan insurance.HealthContext) (insurance.CareAdvice, error) {
	f.calls++
	f.context, f.query = plan, query
	return f.advice, f.err
}

func TestDirectorySearchAppliesConsentedAIRankingWithoutChangingSourceIndices(t *testing.T) {
	for _, scenario := range []struct {
		name, consent, status string
		failure               bool
	}{
		{"ranked", "true", "completed", false},
		{"no_consent", "false", "not_requested", false},
		{"provider_failure", "true", "unavailable", true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			original := http.DefaultTransport
			http.DefaultTransport = careRegistryTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Hostname() != "npiregistry.cms.hhs.gov" {
					return original.RoundTrip(r)
				}
				const registry = `{"results":[{"number":1234567890,"enumeration_type":"NPI-2","basic":{"organization_name":"Alpha Clinic","status":"A"},"addresses":[{"address_purpose":"LOCATION","address_1":"1 Main St","city":"Philadelphia","state":"PA"}],"taxonomies":[{"desc":"Radiology","primary":true}]},{"number":1234567891,"enumeration_type":"NPI-2","basic":{"organization_name":"Zulu Imaging","status":"A"},"addresses":[{"address_purpose":"LOCATION","address_1":"2 Main St","city":"Philadelphia","state":"PA"}],"taxonomies":[{"desc":"Radiology","primary":true}]}]}`
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(registry))}, nil
			})
			t.Cleanup(func() { http.DefaultTransport = original })
			s := supabaseTestService(t)
			ai := &directoryRankingAI{advice: insurance.CareAdvice{Summary: "Compare the supplied listings", Recommendations: []insurance.CareAdviceItem{{Index: 1, Explanation: "Matches the requested imaging service"}, {Index: 0, Explanation: "Another sourced imaging option"}}, Limitations: []string{"Network status unverified"}}}
			if scenario.failure {
				ai.err = errors.New("Synthetic provider unavailable")
			}
			s.healthAI = ai
			r := httptest.NewRequest("GET", "/care/search?q=MRI+knee&city=Philadelphia&state=PA&type=all&ai_consent="+scenario.consent, nil)
			r.Header.Set("Authorization", "Bearer first")
			w := httptest.NewRecorder()
			s.SearchCare(w, r)
			if w.Code != 200 {
				t.Fatalf("directory request failed: %d %s", w.Code, w.Body.String())
			}
			var result struct {
				Results []map[string]any `json:"results"`
				Ranking struct {
					Status string `json:"status"`
				} `json:"ranking"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if len(result.Results) != 2 || result.Ranking.Status != scenario.status {
				t.Fatalf("unexpected result: %s", w.Body.String())
			}
			if result.Results[0]["name"] != "Alpha Clinic" || result.Results[1]["name"] != "Zulu Imaging" {
				t.Fatal("source indices changed during ranking")
			}
			if scenario.status == "completed" {
				if result.Results[0]["ai_rank"] != float64(2) || result.Results[1]["ai_rank"] != float64(1) || ai.query != "MRI knee" || !ai.context.Limited {
					t.Fatal("AI rank, service query or missing-SBC fallback was lost")
				}
			} else if _, ranked := result.Results[0]["ai_rank"]; ranked {
				t.Fatal("failed or unconsented ranking invented ranks")
			}
			if scenario.consent == "false" && ai.calls != 0 {
				t.Fatal("AI was called without consent")
			}
		})
	}
}

func TestCareRankingRejectsUnsupportedIndicesBeforeMutatingRows(t *testing.T) {
	for _, recommendations := range [][]insurance.CareAdviceItem{
		{}, {{Index: 2, Explanation: "Out of range"}}, {{Index: 0, Explanation: "First"}, {Index: 0, Explanation: "Duplicate"}}, {{Index: 0, Explanation: " "}},
	} {
		rows := []map[string]any{{"name": "Actual provider"}}
		if applyCareRanking(rows, insurance.CareAdvice{Recommendations: recommendations}) {
			t.Fatal("unsupported ranking accepted")
		}
		if _, exists := rows[0]["ai_rank"]; exists {
			t.Fatal("invalid ranking changed directory results")
		}
	}
}

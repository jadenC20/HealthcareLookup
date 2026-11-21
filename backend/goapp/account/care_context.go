package account

import (
	"clearcare/caresearch"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	"clearcare/insurance"
)

// SearchCare keeps the directory useful without an SBC or AI response. Plan
// guidance is an explicitly consented enhancement, never a network assertion.
func (s *Service) SearchCare(w http.ResponseWriter, r *http.Request) {
	u, ok := s.identity(w, r, false)
	if !ok {
		return
	}
	recorder := httptest.NewRecorder()
	caresearch.SearchDirectory(recorder, r)
	if recorder.Code != http.StatusOK {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(recorder.Code)
		w.Write(recorder.Body.Bytes())
		return
	}
	var result map[string]any
	if json.Unmarshal(recorder.Body.Bytes(), &result) != nil {
		fail(w, 502, "The provider search returned an unreadable response.")
		return
	}
	plan := s.healthContext(u)
	result["insurance_context"] = plan
	result["ranking"] = map[string]any{"status": "not_requested", "ranked_count": 0}
	if r.URL.Query().Get("ai_consent") == "true" {
		rows := []map[string]any{}
		if items, ok := result["results"].([]any); ok {
			for _, item := range items {
				if row, ok := item.(map[string]any); ok {
					rows = append(rows, row)
				}
				if len(rows) >= 20 {
					break
				}
			}
		}
		if len(rows) > 0 {
			// Use the shared account AI limit without discarding directory results.
			limit := httptest.NewRecorder()
			if !s.allowHealthcareAI(limit, u) {
				result["ai"] = map[string]any{"status": "unavailable", "detail": "Plan guidance request limit reached. Provider results are still available."}
				result["ranking"] = map[string]any{"status": "unavailable", "ranked_count": 0}
			} else if advice, err := s.healthAI.RankCare(r.Context(), strings.TrimSpace(r.URL.Query().Get("q")), rows, plan); err != nil {
				result["ai"] = map[string]any{"status": "unavailable", "detail": err.Error()}
				result["ranking"] = map[string]any{"status": "unavailable", "ranked_count": 0}
			} else if !applyCareRanking(rows, advice) {
				result["ai"] = map[string]any{"status": "unavailable", "detail": "AI did not return a supported ranking. Directory results remain available."}
				result["ranking"] = map[string]any{"status": "unavailable", "ranked_count": 0}
			} else {
				result["ai"] = map[string]any{"status": "completed", "result": advice}
				result["ranking"] = map[string]any{"status": "completed", "ranked_count": len(advice.Recommendations), "evaluated_count": len(rows)}
			}
		} else {
			result["ai"] = map[string]any{"status": "no_data", "detail": "No sourced providers were found to rank. Try a broader service or location."}
			result["ranking"] = map[string]any{"status": "no_data", "ranked_count": 0}
		}
	}
	respond(w, 200, result)
}

// Preserve registry order so advice listing indices still refer to the same
// records. The frontend sorts the explicit rank without changing source IDs.
func applyCareRanking(rows []map[string]any, advice insurance.CareAdvice) bool {
	if len(advice.Recommendations) == 0 || len(advice.Recommendations) > 10 {
		return false
	}
	seen := map[int]bool{}
	for _, item := range advice.Recommendations {
		if item.Index < 0 || item.Index >= len(rows) || seen[item.Index] || strings.TrimSpace(item.Explanation) == "" {
			return false
		}
		seen[item.Index] = true
	}
	for rank, item := range advice.Recommendations {
		rows[item.Index]["ai_rank"] = rank + 1
		rows[item.Index]["ai_reason"] = item.Explanation
	}
	return true
}

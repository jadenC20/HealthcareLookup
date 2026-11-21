package account

import (
	"clearcare/caresearch"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
			} else if advice, err := s.healthAI.ExplainCare(r.Context(), rows, plan); err != nil {
				result["ai"] = map[string]any{"status": "unavailable", "detail": err.Error()}
			} else {
				result["ai"] = map[string]any{"status": "completed", "result": advice}
			}
		}
	}
	respond(w, 200, result)
}

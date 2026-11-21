package insurance

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func response(text string) map[string]any {
	return map[string]any{"status": "completed", "output": []any{map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": text}}}}}
}
func TestOpenAIFileInputAndStrictSchema(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		if req["store"] != false || r.Header.Get("Authorization") != "Bearer fake" {
			t.Error("missing privacy setting/auth")
		}
		format := req["text"].(map[string]any)["format"].(map[string]any)
		if format["strict"] != true || format["type"] != "json_schema" {
			t.Error("missing schema")
		}
		raw, _ := json.Marshal(req["input"])
		if !strings.Contains(string(raw), "input_file") || !strings.Contains(string(raw), "data:application/pdf;base64,") {
			t.Error("PDF not attached correctly")
		}
		b, _ := json.Marshal(Extraction{Details: testDetails(), Unreadable: []string{}})
		json.NewEncoder(w).Encode(response(string(b)))
	}))
	defer server.Close()
	a := &OpenAI{Key: "fake", Model: "test", client: server.Client(), endpoint: server.URL}
	x, err := a.Extract(context.Background(), NewDocument([]byte("%PDF-fixture"), "application/pdf", "uploaded"))
	if err != nil || x.Details.PlanID != testDetails().PlanID {
		t.Fatalf("%+v %v", x, err)
	}
}
func TestSearchRejectsInventedURL(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		if calls == 1 {
			if req["tool_choice"] != "required" {
				t.Error("search not required")
			}
			out := response("A plan document was found.")
			out["output"] = append(out["output"].([]any), map[string]any{"type": "web_search_call", "action": map[string]any{"sources": []any{map[string]any{"url": "https://insurer.example/real.pdf"}}}})
			json.NewEncoder(w).Encode(out)
			return
		}
		json.NewEncoder(w).Encode(response(`{"candidates":[{"details":{"insurer":"Example","plan_name":"Gold","plan_id":"","network":"","state":"PA","year":2026,"employer":""},"url":"https://insurer.example/invented.pdf","source_url":"https://insurer.example/real.pdf"}]}`))
	}))
	defer server.Close()
	a := &OpenAI{Key: "fake", Model: "test", SearchModel: "test", client: server.Client(), endpoint: server.URL}
	c, err := a.Discover(context.Background(), testDetails())
	if err != nil || len(c) != 0 {
		t.Fatalf("invented document accepted: %+v %v", c, err)
	}
}
func TestOpenAIFailures(t *testing.T) {
	for _, status := range []int{401, 429, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				w.Write([]byte("secret provider detail"))
			}))
			defer server.Close()
			a := &OpenAI{Key: "fake", client: server.Client(), endpoint: server.URL}
			_, err := a.Extract(context.Background(), Document{})
			if err == nil || strings.Contains(err.Error(), "secret") {
				t.Fatalf("unsafe error: %v", err)
			}
		})
	}
	for _, res := range []map[string]any{{"status": "incomplete"}, response(""), {"status": "completed", "output": []any{map[string]any{"content": []any{map[string]any{"type": "refusal", "refusal": "no"}}}}}} {
		if _, _, err := responseText(res); err == nil {
			t.Fatal("unusable output accepted")
		}
	}
}

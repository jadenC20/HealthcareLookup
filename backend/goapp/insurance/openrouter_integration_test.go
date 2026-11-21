package insurance

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

type insuranceRoundTripFunc func(*http.Request) (*http.Response, error)

func (f insuranceRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func openRouterFixtureAI(t *testing.T, replies []map[string]any, inspect func(map[string]any)) *OpenAI {
	t.Helper()
	t.Setenv("CLEARCARE_AI_PROVIDER", "openrouter")
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test-fixture")
	t.Setenv("OPENROUTER_MODEL", "")
	ai := NewOpenAI()
	calls := 0
	ai.client = &http.Client{Transport: insuranceRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://openrouter.ai/api/v1/responses" || request.Header.Get("Authorization") != "Bearer sk-or-test-fixture" {
			t.Error("request did not use the selected OpenRouter endpoint and credential")
		}
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if inspect != nil {
			inspect(payload)
		}
		if calls >= len(replies) {
			t.Fatal("unexpected extra provider request")
		}
		raw, err := json.Marshal(replies[calls])
		if err != nil {
			t.Fatal(err)
		}
		calls++
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
	})}
	t.Cleanup(func() {
		if calls != len(replies) {
			t.Errorf("provider requests: got %d, want %d", calls, len(replies))
		}
	})
	return ai
}

func fixtureResponse(t *testing.T, value any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return response(string(raw))
}

func TestAIProviderSelection(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test-fixture")
	t.Setenv("OPENAI_API_KEY", "openai-test-fixture")
	t.Setenv("OPENROUTER_MODEL", "")
	t.Setenv("OPENAI_MODEL", "")
	t.Setenv("OPENAI_SEARCH_MODEL", "")
	t.Setenv("CLOUDFLARE_AI_MODEL", "")
	t.Setenv("CLOUDFLARE_AI_URL", "")
	t.Setenv("CLEARCARE_PROXY_SECRET", "cloudflare-test-fixture")
	for _, test := range []struct{ configured, provider, model string }{
		{"", "cloudflare", "@cf/meta/llama-4-scout-17b-16e-instruct"},
		{"cloudflare", "cloudflare", "@cf/meta/llama-4-scout-17b-16e-instruct"},
		{"openrouter", "openrouter", "deepseek/deepseek-v4.1-flash"},
		{" OPENAI ", "openai", "gpt-4.1"},
	} {
		t.Setenv("CLEARCARE_AI_PROVIDER", test.configured)
		ai := NewOpenAI()
		if ai.Provider != test.provider || ai.Model != test.model {
			t.Fatalf("provider %q selected %q/%q", test.configured, ai.Provider, ai.Model)
		}
	}
}

func TestOpenRouterStructuredUsesJSONModeWithoutReasoningRequirement(t *testing.T) {
	out := Extraction{Details: testDetails(), Unreadable: []string{}}
	ai := openRouterFixtureAI(t, []map[string]any{fixtureResponse(t, out)}, func(payload map[string]any) {
		format := payload["text"].(map[string]any)["format"].(map[string]any)
		if !reflect.DeepEqual(format, map[string]any{"type": "json_object"}) {
			t.Errorf("unexpected JSON format: %#v", format)
		}
		if _, exists := payload["reasoning"]; exists {
			t.Error("unnecessary reasoning support was required from the selected endpoint")
		}
		if !strings.Contains(payload["instructions"].(string), `"required"`) {
			t.Error("output schema was not included in instructions")
		}
	})
	got, err := ai.Extract(context.Background(), NewDocument([]byte("%PDF-fixture"), "application/pdf", "uploaded"))
	if err != nil || got.Details.PlanID != out.Details.PlanID {
		t.Fatalf("structured request failed: %+v %v", got, err)
	}
}

func TestOpenRouterDiscoveryUsesCitationMetadataAndRejectsInventedURL(t *testing.T) {
	docURL, sourceURL := "https://insurer.example/real.pdf", "https://insurer.example/plans"
	search := map[string]any{"status": "completed", "output": []any{map[string]any{
		"type":        "message",
		"annotations": []any{map[string]any{"type": "url_citation", "url_citation": map[string]any{"url": sourceURL}}},
		"content": []any{map[string]any{"type": "output_text", "text": "An SBC was found. https://insurer.example/invented.pdf is unverified.", "annotations": []any{
			map[string]any{"type": "url_citation", "url_citation": map[string]any{"url": docURL}},
			map[string]any{"type": "url_citation", "url": docURL},
		}}},
	}}}
	parsed := map[string]any{"candidates": []any{
		map[string]any{"details": testDetails(), "url": docURL, "source_url": sourceURL},
		map[string]any{"details": testDetails(), "url": "https://insurer.example/invented.pdf", "source_url": sourceURL},
	}}
	calls := 0
	ai := openRouterFixtureAI(t, []map[string]any{search, fixtureResponse(t, parsed)}, func(payload map[string]any) {
		calls++
		if calls == 1 {
			if _, exists := payload["tools"]; exists {
				t.Error("OpenAI web_search tool was not translated")
			}
			plugins := payload["plugins"].([]any)
			if plugins[0].(map[string]any)["id"] != "web" {
				t.Error("OpenRouter web search plugin missing")
			}
		}
	})
	got, err := ai.Discover(context.Background(), testDetails())
	if err != nil || len(got) != 1 || got[0].URL != docURL || got[0].SourceURL != sourceURL {
		t.Fatalf("source-backed discovery failed: %+v %v", got, err)
	}
	_, sources, err := responseText(search)
	if err != nil || !reflect.DeepEqual(sources, []string{sourceURL, docURL}) {
		t.Fatalf("citation metadata lost or duplicated: %#v %v", sources, err)
	}
}

func TestOpenRouterHealthcareSchemas(t *testing.T) {
	bill := BillExtraction{DocumentType: "medical_bill", Missing: []string{}, Warnings: []string{}}
	bill.Bill.Lines = []ExtractedBillLine{{ID: "L1", Description: "Visit", Page: 1, Quote: "Visit"}}
	advice := BillAdvice{Summary: "Review the visit", Items: []BillAdviceItem{{Lines: []string{"L1"}, Explanation: "Confirm the item", Question: "What is included?"}}, Inquiry: "Please itemize.", Limitations: []string{}}
	care := CareAdvice{Summary: "Compare these records", Recommendations: []CareAdviceItem{{Index: 0, Explanation: "Confirm participation", Questions: []string{}, Benefits: []int{}}}, Limitations: []string{}}
	interpretation := Interpretation{IsSBC: true, Details: testDetails(), Benefits: allBenefits(), Missing: []string{}}
	for _, test := range []struct {
		name   string
		output any
		call   func(*OpenAI) error
	}{
		{"bill", bill, func(ai *OpenAI) error { _, err := ai.ExtractBill(context.Background(), Document{}); return err }},
		{"bill_advice", advice, func(ai *OpenAI) error {
			_, err := ai.ReviewBill(context.Background(), map[string]any{"bill": map[string]any{"lines": []any{map[string]any{"line_id": "L1"}}}}, map[string]any{}, HealthContext{})
			return err
		}},
		{"care", care, func(ai *OpenAI) error {
			_, err := ai.ExplainCare(context.Background(), []map[string]any{{"provider_id": "P1"}}, HealthContext{})
			return err
		}},
		{"sbc", interpretation, func(ai *OpenAI) error { _, err := ai.Interpret(context.Background(), Document{}); return err }},
	} {
		t.Run(test.name, func(t *testing.T) {
			ai := openRouterFixtureAI(t, []map[string]any{fixtureResponse(t, test.output)}, nil)
			if err := test.call(ai); err != nil {
				t.Fatalf("application schema was rejected: %v", err)
			}
		})
	}
}

func TestOpenRouterRejectsStructurallyIncompleteJSON(t *testing.T) {
	ai := openRouterFixtureAI(t, []map[string]any{fixtureResponse(t, map[string]any{"details": testDetails()})}, nil)
	if _, err := ai.Extract(context.Background(), Document{}); err == nil {
		t.Fatal("missing required fields were accepted as zero values")
	}
}

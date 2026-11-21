package insurance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func cloudflareFixtureAI(t *testing.T, reply map[string]any, inspect func(map[string]any)) *OpenAI {
	t.Helper()
	t.Setenv("CLEARCARE_AI_PROVIDER", "cloudflare")
	t.Setenv("CLEARCARE_PROXY_SECRET", "bridge-test-secret")
	t.Setenv("CLOUDFLARE_AI_MODEL", "")
	t.Setenv("CLOUDFLARE_AI_URL", "")
	ai := NewOpenAI()
	ai.client.Transport = insuranceRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://clearcare-ai.agrigine-mcp.workers.dev/internal/ai" || request.Method != http.MethodPost {
			t.Error("AI request did not use the private Cloudflare bridge")
		}
		if request.Header.Get("X-ClearCare-Proxy-Secret") != "bridge-test-secret" || request.Header.Get("Authorization") != "" {
			t.Error("Cloudflare bridge authentication headers are incorrect")
		}
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["model"] != "@cf/meta/llama-4-scout-17b-16e-instruct" || payload["store"] != false {
			t.Error("Cloudflare model or non-storage request setting changed")
		}
		if _, exists := payload["provider"]; exists {
			t.Error("OpenRouter provider routing leaked into Cloudflare payload")
		}
		if inspect != nil {
			inspect(payload)
		}
		raw, err := json.Marshal(reply)
		if err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
	})
	return ai
}

func TestCloudflareBridgeDocumentAndSchema(t *testing.T) {
	ai := cloudflareFixtureAI(t, fixtureResponse(t, Extraction{Details: testDetails(), Unreadable: []string{}}), func(payload map[string]any) {
		format := payload["text"].(map[string]any)["format"].(map[string]any)
		if format["type"] != "json_object" || !strings.Contains(payload["instructions"].(string), `"required"`) {
			t.Error("JSON mode or schema instructions missing")
		}
		input := payload["input"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)
		if input["type"] != "input_file" || !strings.HasPrefix(input["file_data"].(string), "data:application/pdf;base64,") {
			t.Error("document was not passed to the bridge")
		}
	})
	got, err := ai.Extract(context.Background(), NewDocument([]byte("%PDF-fixture"), "application/pdf", "uploaded"))
	if err != nil || got.Details.PlanID != testDetails().PlanID {
		t.Fatalf("Cloudflare extraction failed: %+v %v", got, err)
	}
}

func TestCloudflareBridgePreservesSearchToolAndSources(t *testing.T) {
	const sourceURL = "https://insurer.example/plan.pdf"
	reply := response("Official plan document")
	reply["output"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)["annotations"] = []any{map[string]any{"type": "url_citation", "url": sourceURL}}
	ai := cloudflareFixtureAI(t, reply, func(payload map[string]any) {
		tools, ok := payload["tools"].([]any)
		if !ok || len(tools) != 1 || tools[0].(map[string]any)["type"] != "web_search" || payload["tool_choice"] != "required" {
			t.Error("search request was not passed to the Cloudflare bridge")
		}
		if _, exists := payload["plugins"]; exists {
			t.Error("OpenRouter search plugin leaked into Cloudflare payload")
		}
	})
	got, err := ai.Search(context.Background(), "Find official plan documents")
	if err != nil || len(got.Sources) != 1 || got.Sources[0] != sourceURL {
		t.Fatalf("Cloudflare sources were lost: %+v %v", got, err)
	}
}

func TestCloudflareBridgeRejectsIncompleteStructuredOutput(t *testing.T) {
	ai := cloudflareFixtureAI(t, fixtureResponse(t, map[string]any{"details": testDetails()}), nil)
	if _, err := ai.Extract(context.Background(), Document{}); err == nil {
		t.Fatal("Cloudflare output bypassed local required-field validation")
	}
}

func TestCloudflareBridgeFailuresDoNotExposeProviderDetails(t *testing.T) {
	for _, status := range []int{401, 403, 429, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			ai := cloudflareFixtureAI(t, nil, nil)
			ai.client.Transport = insuranceRoundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("private-provider-detail"))}, nil
			})
			_, err := ai.Search(context.Background(), "public query")
			if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("cloudflare_http_%d", status)) || strings.Contains(err.Error(), "private-provider-detail") || strings.Contains(err.Error(), "bridge-test-secret") {
				t.Fatalf("unsafe Cloudflare error: %v", err)
			}
		})
	}
}

func TestCloudflareBridgeMissingSecretAndRedirectPolicy(t *testing.T) {
	t.Setenv("CLEARCARE_AI_PROVIDER", "cloudflare")
	t.Setenv("CLEARCARE_PROXY_SECRET", "")
	ai := NewOpenAI()
	if _, err := ai.Search(context.Background(), "public query"); err == nil || !strings.Contains(err.Error(), "ai_not_configured") {
		t.Fatal("missing bridge secret did not produce a configuration error")
	}
	if ai.client.CheckRedirect == nil || ai.client.CheckRedirect(nil, nil) != http.ErrUseLastResponse {
		t.Fatal("bridge redirects could forward its private authentication header")
	}
}

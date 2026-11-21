package insurance

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func healthcareTestAI(t *testing.T, output any, inspect func(map[string]any)) *OpenAI {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input map[string]any
		json.NewDecoder(r.Body).Decode(&input)
		if inspect != nil {
			inspect(input)
		}
		raw, _ := json.Marshal(output)
		json.NewEncoder(w).Encode(response(string(raw)))
	}))
	t.Cleanup(server.Close)
	return &OpenAI{Key: "fake", Model: "test", client: server.Client(), endpoint: server.URL}
}
func TestBillExtractionKeepsUnknownAmountsUnknown(t *testing.T) {
	out := BillExtraction{DocumentType: "medical_bill", Missing: []string{"quantity"}, Warnings: []string{}}
	out.Bill.Provider = "Clinic"
	out.Bill.Lines = []ExtractedBillLine{{ID: "L1", Description: "Visit", Page: 1, Quote: "Visit"}}
	ai := healthcareTestAI(t, out, nil)
	result, err := ai.ExtractBill(context.Background(), NewDocument([]byte("%PDF-synthetic"), "application/pdf", "uploaded"))
	if err != nil || result.Bill.Lines[0].Charge != nil || result.Bill.Lines[0].Quantity != nil {
		t.Fatalf("missing amounts were guessed: %+v %v", result, err)
	}
}
func TestBillExplanationDoesNotSendAccountIdentifiersAndRequiresRealLines(t *testing.T) {
	req := map[string]any{"patient_name": "PRIVATE_PATIENT", "account_number": "PRIVATE_ACCOUNT", "bill": map[string]any{"provider_name": "Clinic", "claim_number": "PRIVATE_CLAIM", "lines": []any{map[string]any{"line_id": "L1", "description": "Visit", "quantity": 1, "charge": 75}}}}
	for _, id := range []string{"L1", "invented"} {
		out := BillAdvice{Summary: "Explain the visit", Items: []BillAdviceItem{{Lines: []string{id}, Explanation: "Ask what is included", Question: "Is this itemized?"}}, Inquiry: "Please itemize.", Limitations: []string{"Coverage unverified"}}
		ai := healthcareTestAI(t, out, func(input map[string]any) {
			raw, _ := json.Marshal(input["input"])
			for _, private := range []string{"PRIVATE_PATIENT", "PRIVATE_ACCOUNT", "PRIVATE_CLAIM"} {
				if strings.Contains(string(raw), private) {
					t.Error("unnecessary identifier sent to AI")
				}
			}
		})
		_, err := ai.ReviewBill(context.Background(), req, map[string]any{"findings": []any{}}, HealthContext{})
		if (err != nil) != (id == "invented") {
			t.Fatal(id, err)
		}
	}
}
func TestCareGuidanceRejectsInventedListingsBenefitsAndFinancialClaims(t *testing.T) {
	for _, item := range []CareAdviceItem{{Index: 0, Explanation: "Confirm network participation", Benefits: []int{0}}, {Index: 9}, {Index: 0, Benefits: []int{4}}, {Index: 0, Explanation: "You owe $25"}} {
		ai := healthcareTestAI(t, CareAdvice{Summary: "Compare sourced records", Recommendations: []CareAdviceItem{item}, Limitations: []string{}}, nil)
		_, err := ai.ExplainCare(context.Background(), []map[string]any{{"provider_id": "P1", "price": 75}}, HealthContext{Benefits: []Benefit{{Name: "Office visit", Quote: "Refer to plan", Page: 1}}})
		shouldPass := item.Index == 0 && item.Explanation == "Confirm network participation"
		if (err == nil) != shouldPass {
			t.Fatal(item, err)
		}
	}
}

func TestCloudflareBillExplanationSchemaRequiresExistingLineReferences(t *testing.T) {
	req := map[string]any{"bill": map[string]any{"lines": []any{map[string]any{"line_id": "L2"}, map[string]any{"line_id": "L1"}, map[string]any{"line_id": "L1"}}}}
	for _, references := range [][]string{{"L1"}, {}, {"invented"}} {
		out := BillAdvice{Summary: "Review the bill", Items: []BillAdviceItem{{Lines: references, Explanation: "Confirm this item", Question: "What is included?"}}, Inquiry: "Please itemize.", Limitations: []string{}}
		ai := cloudflareFixtureAI(t, fixtureResponse(t, out), func(payload map[string]any) {
			format := payload["text"].(map[string]any)["format"].(map[string]any)
			schema := format["schema"].(map[string]any)
			properties := schema["properties"].(map[string]any)
			item := properties["items"].(map[string]any)["items"].(map[string]any)
			lineIDs := item["properties"].(map[string]any)["line_ids"].(map[string]any)
			if lineIDs["minItems"] != float64(1) || !reflect.DeepEqual(lineIDs["items"].(map[string]any)["enum"], []any{"L1", "L2"}) {
				t.Errorf("grounded references were not constrained in the generation schema: %#v", lineIDs)
			}
			if !strings.Contains(payload["instructions"].(string), "general advice and missing-evidence notes in limitations") {
				t.Error("general advice was not directed outside line-specific items")
			}
		})
		_, err := ai.ReviewBill(context.Background(), req, map[string]any{}, HealthContext{})
		if (err == nil) != (len(references) == 1 && references[0] == "L1") {
			t.Fatalf("reference validation result: references=%v err=%v", references, err)
		}
	}
}

func TestBillExplanationWithoutGroundedItemsCanReturnLimitations(t *testing.T) {
	req := map[string]any{"bill": map[string]any{"lines": []any{map[string]any{"line_id": "L1"}}}}
	out := BillAdvice{Summary: "More evidence is needed", Items: []BillAdviceItem{}, Inquiry: "Please send itemized detail.", Limitations: []string{"Confirm the bill with the provider."}}
	ai := cloudflareFixtureAI(t, fixtureResponse(t, out), nil)
	if _, err := ai.ReviewBill(context.Background(), req, map[string]any{}, HealthContext{}); err != nil {
		t.Fatalf("general advice required fabricated citations: %v", err)
	}
}

func TestBillExplanationRejectsMissingInputLineIdentifiers(t *testing.T) {
	ai := &OpenAI{}
	if _, err := ai.ReviewBill(context.Background(), map[string]any{"bill": map[string]any{"lines": []any{map[string]any{"description": "Visit"}}}}, map[string]any{}, HealthContext{}); err == nil {
		t.Fatal("missing input identifiers reached AI generation")
	}
}

func TestCloudflareRankCareUsesServiceQueryAndOriginalSourceIndicesWithoutBenefits(t *testing.T) {
	rows := []map[string]any{
		{"name": "General practice", "specialty": "Internal medicine", "city": "Albany", "source_url": "https://directory.example/general"},
		{"name": "Eye practice", "specialty": "Ophthalmology", "city": "Albany", "source_url": "https://directory.example/eyes"},
	}
	want := CareAdvice{Summary: "Sourced matches for your search", Recommendations: []CareAdviceItem{
		{Index: 1, Explanation: "The directory lists this practice's specialty as ophthalmology in Albany.", Questions: []string{"Confirm the practice offers the requested service."}, Benefits: []int{}},
		{Index: 0, Explanation: "This directory record is a broader specialty match.", Questions: []string{}, Benefits: []int{}},
	}, Limitations: []string{"Network participation and clinical quality are unverified."}}
	ai := cloudflareFixtureAI(t, fixtureResponse(t, want), func(payload map[string]any) {
		content := payload["input"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
		var evidence map[string]any
		if err := json.Unmarshal([]byte(content), &evidence); err != nil || evidence["service_query"] != "eye exam in Albany" {
			t.Errorf("service query missing from ranking evidence: %s, %v", content, err)
		}
		listings := evidence["listings"].([]any)
		if listings[1].(map[string]any)["source_url"] != rows[1]["source_url"] {
			t.Error("ranking did not retain source evidence at the original index")
		}
		prompt := payload["instructions"].(string)
		for _, instruction := range []string{"Rank recommendations in order of best match", "A saved SBC or documented benefits are not required", "not its rank position"} {
			if !strings.Contains(prompt, instruction) {
				t.Errorf("ranking instruction missing: %s", instruction)
			}
		}
		format := payload["text"].(map[string]any)["format"].(map[string]any)
		properties := format["schema"].(map[string]any)["properties"].(map[string]any)
		recommendations := properties["recommendations"].(map[string]any)
		if recommendations["minItems"] != float64(1) || recommendations["maxItems"] != float64(10) {
			t.Errorf("ranking output size is unconstrained: %#v", recommendations)
		}
		item := recommendations["items"].(map[string]any)["properties"].(map[string]any)
		if !reflect.DeepEqual(item["listing_index"].(map[string]any)["enum"], []any{float64(0), float64(1)}) || item["benefit_indices"].(map[string]any)["maxItems"] != float64(0) {
			t.Errorf("ranking references are unconstrained: %#v", item)
		}
	})
	got, err := ai.RankCare(context.Background(), "eye exam in Albany", rows, HealthContext{Benefits: []Benefit{}, Limited: true})
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("ranking lost grounded source order: got=%+v err=%v", got, err)
	}
}

func TestCloudflareRankCareRejectsUnsupportedReferencesAndEmptyRanking(t *testing.T) {
	for name, recommendations := range map[string][]CareAdviceItem{
		"unknown_listing":   {{Index: 2, Benefits: []int{}, Questions: []string{}}},
		"negative_listing":  {{Index: -1, Benefits: []int{}, Questions: []string{}}},
		"invented_benefit":  {{Index: 0, Benefits: []int{0}, Questions: []string{}}},
		"duplicate_listing": {{Index: 0, Benefits: []int{}, Questions: []string{}}, {Index: 0, Benefits: []int{}, Questions: []string{}}},
		"no_ranking":        {},
	} {
		t.Run(name, func(t *testing.T) {
			out := CareAdvice{Summary: "Search matches", Recommendations: recommendations, Limitations: []string{}}
			ai := cloudflareFixtureAI(t, fixtureResponse(t, out), nil)
			if _, err := ai.RankCare(context.Background(), "eye exam", []map[string]any{{"name": "Eye practice", "source_url": "https://directory.example/eyes"}}, HealthContext{Benefits: []Benefit{}}); err == nil {
				t.Fatal("unsupported care ranking accepted")
			}
		})
	}
}

func TestCloudflareCareGuidanceConstrainsKnownBenefitIndicesAndAllowsNoRankings(t *testing.T) {
	plan := HealthContext{Benefits: []Benefit{{Name: "specialist", Value: "Refer to plan", Page: 1, Quote: "See plan terms"}}}
	for _, indices := range [][]int{{0}, {1}} {
		out := CareAdvice{Summary: "Compare sourced records", Recommendations: []CareAdviceItem{{Index: 0, Explanation: "Confirm the plan restriction.", Questions: []string{}, Benefits: indices}}, Limitations: []string{}}
		ai := cloudflareFixtureAI(t, fixtureResponse(t, out), func(payload map[string]any) {
			format := payload["text"].(map[string]any)["format"].(map[string]any)
			recommendations := format["schema"].(map[string]any)["properties"].(map[string]any)["recommendations"].(map[string]any)
			if _, exists := recommendations["minItems"]; exists {
				t.Error("ExplainCare unexpectedly requires ranking output")
			}
			benefits := recommendations["items"].(map[string]any)["properties"].(map[string]any)["benefit_indices"].(map[string]any)
			if !reflect.DeepEqual(benefits["items"].(map[string]any)["enum"], []any{float64(0)}) {
				t.Error("known benefit references missing from schema")
			}
		})
		_, err := ai.ExplainCare(context.Background(), []map[string]any{{"name": "Sourced practice"}}, plan)
		if (err == nil) != (indices[0] == 0) {
			t.Fatalf("benefit grounding failed: indices=%v err=%v", indices, err)
		}
	}
	ai := cloudflareFixtureAI(t, fixtureResponse(t, CareAdvice{Summary: "No sourced matches", Recommendations: []CareAdviceItem{}, Limitations: []string{}}), nil)
	if _, err := ai.RankCare(context.Background(), "eye exam", nil, HealthContext{}); err != nil {
		t.Fatalf("empty source list required an invented ranking: %v", err)
	}
}

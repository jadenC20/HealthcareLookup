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

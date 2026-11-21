package insurance

import (
	"clearcare/billreview"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	req := billreview.Request{Patient: "PRIVATE_PATIENT", Account: "PRIVATE_ACCOUNT", Bill: billreview.Bill{Provider: "Clinic", Claim: "PRIVATE_CLAIM", Lines: []billreview.Line{{ID: "L1", Description: "Visit", Quantity: 1, Charge: 75}}}}
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
		_, err := ai.ReviewBill(context.Background(), req, billreview.Analyze(req), HealthContext{})
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

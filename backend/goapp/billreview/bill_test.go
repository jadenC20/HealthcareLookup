package billreview

import (
	"encoding/json"
	"os"
	"testing"
)

func TestSampleAnalysis(t *testing.T) {
	data, err := os.ReadFile("../../sample/bill_with_problems.json")
	if err != nil {
		t.Fatal(err)
	}
	var req Request
	if err = json.Unmarshal(data, &req); err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, f := range Analyze(req)["findings"].([]Finding) {
		found[f.Rule] = true
	}
	for _, rule := range []string{"possible_duplicate", "unclear_fee", "billed_amount_mismatch", "patient_responsibility_mismatch"} {
		if !found[rule] {
			t.Errorf("missing %s", rule)
		}
	}
}
func TestUnknownEOBTotalIsNotZero(t *testing.T) {
	amount := 100.0
	r := Request{Bill: Bill{Provider: "Test", Patient: &amount}, EOB: &EOB{}}
	for _, f := range Analyze(r)["findings"].([]Finding) {
		if f.Rule == "patient_responsibility_mismatch" {
			t.Fatal("missing amount treated as zero")
		}
	}
}

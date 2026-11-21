package billreview

import (
	"clearcare/evidence"
	"clearcare/insurance"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type CodingEdit struct {
	Code1             string `json:"code1"`
	Code2             string `json:"code2"`
	Setting           string `json:"setting"`
	Start             string `json:"effective_from"`
	End               string `json:"effective_to"`
	ModifierIndicator int    `json:"modifier_indicator"`
	Source            string `json:"source_url"`
}

func codingFindings(dir string, req ReviewRequest, report *evidence.Report) []Finding {
	out := []Finding{}
	data, err := os.ReadFile(filepath.Join(dir, "ncci_edits.json"))
	var edits []CodingEdit
	if err != nil || len(data) > 32<<20 || json.Unmarshal(data, &edits) != nil || len(edits) == 0 {
		report.Warn("coding_edits", "ncci_data_missing", "Optional NCCI data is unavailable. General bill checks and public coding-reference research continue without asserting code-pair violations.", "Use the bill findings and cited research to ask the biller about coding; no import is required to continue.", "verified_ncci_pair_checks")
		return out
	}
	applicable := false
	for _, edit := range edits {
		start, e1 := time.Parse("2006-01-02", edit.Start)
		end, e2 := time.Parse("2006-01-02", edit.End)
		if e1 != nil || e2 != nil || end.Before(start) || edit.Setting != req.Setting || insurance.SafeURL(edit.Source) != nil || edit.ModifierIndicator < 0 || edit.ModifierIndicator > 1 {
			continue
		}
		for _, a := range req.Request.Bill.Lines {
			date, err := time.Parse("2006-01-02", a.Date)
			if err != nil || date.Before(start) || date.After(end) {
				continue
			}
			applicable = true
			if a.Code != edit.Code1 {
				continue
			}
			for _, b := range req.Request.Bill.Lines {
				if b.ID == a.ID || b.Code != edit.Code2 || b.Date != a.Date {
					continue
				}
				message := "These codes match an imported NCCI edit for the service date and setting. Confirm claim context before disputing the charge."
				if edit.ModifierIndicator == 1 {
					message = "These codes match a modifier-dependent NCCI edit. A documented exception may apply; no improper-billing conclusion was made."
				}
				out = append(out, Finding{Rule: "ncci_pair_review", Title: "Code pair needs review", Message: message, IDs: []string{a.ID, b.ID}, Evidence: map[string]any{"source_url": edit.Source, "modifier_indicator": edit.ModifierIndicator, "modifiers": append(append([]string{}, a.Modifiers...), b.Modifiers...), "effective_from": edit.Start, "effective_to": edit.End}, Question: "Please explain the code-pair billing and any applicable modifier exception."})
			}
		}
	}
	if !applicable {
		report.Warn("coding_edits", "ncci_release_not_applicable", "No loaded edit release matched the supplied dates and setting.", "Supply the applicable edit release.", "applicable_ncci_release")
	}
	report.Warn("coding_edits", "claim_context_unverified", "NCCI matches are review flags, not claim adjudication; provider, claim type and modifier documentation must be checked.", "Ask the biller to verify the applicable edit and exceptions.", "claim_context", "modifier_documentation")
	return out
}

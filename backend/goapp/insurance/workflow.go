package insurance

import (
	"context"
	"errors"
	"regexp"
	"strings"
)

// Run extracts a new card unless confirmed details are supplied. It never substitutes
// a similarly named plan when identifiers, year, or variant disagree.
func (e *Engine) Run(ctx context.Context, card Document, details *Details) Result {
	r := NewResult(Details{}, details != nil)
	if details != nil {
		r.Details = *details
		r.step("card_extraction", "user_supplied")
	} else {
		extraction, err := e.AI.Extract(ctx, card)
		if err != nil {
			r.failure("card_extraction", err)
			r.Finish()
			return r
		}
		if err = extraction.Details.Validate(); err != nil {
			r.failure("card_extraction", stageError("invalid_extraction", "Extracted plan details were invalid."))
			r.Finish()
			return r
		}
		r.Details = extraction.Details
		r.step("card_extraction", "completed")
		if len(extraction.Unreadable) > 0 {
			r.warn("card_extraction", "unreadable_fields", "Some card fields could not be read.", "Confirm or correct the extracted details.", extraction.Unreadable...)
		}
		r.warn("card_extraction", "unconfirmed_details", "Card details were read by AI and have not been confirmed.", "Confirm the insurer, plan, state and coverage year.", "details_confirmation")
		r.Status = "needs_details_confirmation"
		r.step("discovery", "awaiting_confirmation")
		r.Finish()
		return r
	}
	e.discover(ctx, &r)
	r.Finish()
	return r
}
func (e *Engine) discover(ctx context.Context, r *Result) {
	missing := []string{}
	if r.Details.Insurer == "" {
		missing = append(missing, "insurer")
	}
	if r.Details.PlanID == "" && r.Details.PlanName == "" {
		missing = append(missing, "plan_name_or_id")
	}
	if r.Details.Year == 0 {
		missing = append(missing, "year")
	}
	if r.Details.State == "" {
		missing = append(missing, "state")
	}
	if len(missing) > 0 {
		r.warn("plan_details", "missing_fields", "The card does not establish all details needed for an exact plan match.", "Provide the missing plan details.", missing...)
	}
	if r.Details.Insurer == "" || (r.Details.PlanName == "" && r.Details.PlanID == "") {
		r.step("discovery", "blocked")
		return
	}
	var candidates []Candidate
	var catalogErr error
	if e.Catalog != nil {
		candidates, catalogErr = e.Catalog.Find(ctx, r.Details)
	}
	var stage *StageError
	if e.Catalog == nil || errors.As(catalogErr, &stage) && stage.Code == "catalog_not_configured" {
		// The catalog is optional; online discovery is a supported path.
		r.step("catalog_lookup", "not_configured")
		catalogErr = nil
	} else if catalogErr != nil {
		r.step("catalog_lookup", "unavailable")
	} else {
		r.step("catalog_lookup", "completed")
	}
	candidates = usable(candidates, r.Details)
	if len(candidates) != 1 || candidates[0].Match != "exact_variant" {
		web, err := e.AI.Discover(ctx, r.Details)
		if err != nil {
			r.failure("web_search", err)
		} else {
			r.step("web_search", "completed")
			candidates = append(candidates, usable(web, r.Details)...)
		}
	} else {
		r.step("web_search", "not_needed")
	}
	seen := map[string]bool{}
	for _, c := range candidates {
		if !seen[c.ID] {
			r.Candidates = append(r.Candidates, c)
			seen[c.ID] = true
		}
		if len(r.Candidates) >= 20 {
			break
		}
	}
	if len(r.Candidates) == 0 {
		if catalogErr != nil {
			r.failure("catalog_lookup", catalogErr)
		}
		r.step("plan_match", "missing")
		r.warn("plan_match", "no_candidates", "No supported SBC candidate was found.", "Supply the exact plan details or upload an SBC from your insurer.", "sbc_document")
		return
	}
	if len(r.Candidates) == 1 && r.Candidates[0].Match == "exact_variant" && r.DetailsConfirmed {
		r.Selected = &r.Candidates[0]
		r.step("plan_match", "completed")
		e.readSelected(ctx, r)
		return
	}
	r.Status = "needs_confirmation"
	r.step("plan_match", "needs_confirmation")
	r.warn("plan_match", "ambiguous_or_unconfirmed", "The exact plan and benefit variant could not be established automatically.", "Confirm plan details and select a candidate SBC.", "exact_plan_confirmation")
}

var variantID = regexp.MustCompile(`^\d{5}[A-Z]{2}\d{7}-\d{2}$`)

func usable(candidates []Candidate, d Details) []Candidate {
	out := []Candidate{}
	for _, c := range candidates {
		if SafeURL(c.URL) != nil {
			continue
		}
		if c.Details.Year != 0 && d.Year != 0 && c.Details.Year != d.Year {
			continue
		}
		if c.Details.State != "" && d.State != "" && !strings.EqualFold(c.Details.State, d.State) {
			continue
		}
		if c.Details.PlanID != "" && d.PlanID != "" && c.Details.PlanID != d.PlanID && baseID(c.Details.PlanID) != baseID(d.PlanID) {
			continue
		}
		if variantID.MatchString(d.PlanID) && variantID.MatchString(c.Details.PlanID) && c.Details.PlanID != d.PlanID {
			continue
		}
		c.Match = "candidate"
		if c.Source == "cms_catalog" && variantID.MatchString(d.PlanID) && c.Details.PlanID == d.PlanID && d.Year != 0 && c.Details.Year == d.Year && d.State != "" && c.Details.State == d.State {
			c.Match = "exact_variant"
		}
		c.Identify()
		out = append(out, c)
	}
	return out
}
func baseID(s string) string { return strings.Split(s, "-")[0] }
func (e *Engine) Select(ctx context.Context, previous Result, id string) (Result, error) {
	if !previous.DetailsConfirmed {
		return previous, stageError("details_confirmation_required", "Confirm your plan details before selecting an SBC.")
	}
	r := NewResult(previous.Details, previous.DetailsConfirmed)
	r.Candidates = previous.Candidates
	for _, c := range previous.Candidates {
		if c.ID == id {
			v := c
			r.Selected = &v
			break
		}
	}
	if r.Selected == nil {
		return r, stageError("unknown_candidate", "Select a candidate returned by the current lookup.")
	}
	r.step("plan_match", "user_selected")
	if !r.DetailsConfirmed {
		r.warn("plan_match", "unconfirmed_details", "The user selected a document but the plan details remain unconfirmed.", "Confirm the extracted plan details.", "details_confirmation")
	}
	e.readSelected(ctx, &r)
	r.Finish()
	return r, nil
}
func (e *Engine) readSelected(ctx context.Context, r *Result) {
	doc, err := e.Fetcher.Fetch(ctx, r.Selected.URL)
	if err != nil {
		r.failure("sbc_retrieval", err)
		return
	}
	r.step("sbc_retrieval", "completed")
	e.interpret(ctx, r, doc)
}
func (e *Engine) Upload(ctx context.Context, previous Result, doc Document) Result {
	r := NewResult(previous.Details, previous.DetailsConfirmed)
	r.step("sbc_retrieval", "user_uploaded")
	r.warn("plan_match", "user_supplied_document", "The SBC was supplied by the user; its source and enrollment have not been independently verified.", "Check that the document describes your exact plan and coverage year.", "source_verification")
	e.interpret(ctx, &r, doc)
	r.Finish()
	return r
}

var RequiredBenefits = []string{"deductible", "out_of_pocket_limit", "primary_care", "specialist", "emergency_room", "hospital_stay", "prescription_drugs", "exclusions"}

// Reinterpret retries the saved document without repeating discovery or changing
// the selected source. Failed interpretations never discard the original PDF.
func (e *Engine) Reinterpret(ctx context.Context, previous Result, doc Document) Result {
	if previous.Selected == nil {
		return e.Upload(ctx, previous, doc)
	}
	r := NewResult(previous.Details, previous.DetailsConfirmed)
	r.Selected, r.Candidates = previous.Selected, previous.Candidates
	r.step("sbc_retrieval", "saved_document")
	if !r.DetailsConfirmed {
		r.warn("plan_match", "unconfirmed_details", "Plan details remain unconfirmed.", "Confirm the extracted plan details.", "details_confirmation")
	}
	e.interpret(ctx, &r, doc)
	r.Finish()
	return r
}

func (e *Engine) interpret(ctx context.Context, r *Result, doc Document) {
	r.Document = &doc
	info, err := e.AI.Interpret(ctx, doc)
	if err != nil {
		r.failure("sbc_interpretation", err)
		return
	}
	if !info.IsSBC {
		r.failure("sbc_interpretation", stageError("not_an_sbc", "The document could not be identified as a Summary of Benefits and Coverage."))
		return
	}
	if err := info.Details.Validate(); err != nil {
		r.failure("sbc_interpretation", stageError("invalid_document_identity", "The extracted document identity is invalid."))
		return
	}
	r.DocumentDetails = &info.Details
	mismatches := []string{}
	expected := r.Details
	if r.Selected != nil { // Candidate metadata is also checked when user details are incomplete.
		if expected.Year == 0 {
			expected.Year = r.Selected.Details.Year
		}
		if expected.PlanID == "" {
			expected.PlanID = r.Selected.Details.PlanID
		}
		if expected.State == "" {
			expected.State = r.Selected.Details.State
		}
	}
	if info.Details.Year != 0 && expected.Year != 0 && info.Details.Year != expected.Year {
		mismatches = append(mismatches, "year")
	}
	if info.Details.PlanID != "" && expected.PlanID != "" && info.Details.PlanID != expected.PlanID {
		mismatches = append(mismatches, "plan_id_or_variant")
	}
	if info.Details.State != "" && expected.State != "" && !strings.EqualFold(info.Details.State, expected.State) {
		mismatches = append(mismatches, "state")
	}
	if info.Details.Insurer != "" && expected.Insurer != "" && !similar(info.Details.Insurer, expected.Insurer) {
		mismatches = append(mismatches, "insurer")
	}
	if len(mismatches) > 0 {
		r.step("sbc_interpretation", "conflict")
		r.warn("sbc_interpretation", "document_mismatch", "The SBC conflicts with the expected plan. Benefits have been withheld.", "Select the correct SBC or correct the plan details.", mismatches...)
		return
	}
	if info.Details.PlanID == "" || expected.PlanID == "" || info.Details.Year == 0 || expected.Year == 0 {
		r.warn("plan_match", "document_identity_incomplete", "The document does not establish an exact plan ID and year match.", "Check the document against your insurer's plan details.", "document_plan_id_or_year")
	}
	seen := map[string]bool{}
	for _, b := range info.Benefits {
		if b.Value == "" || b.Page < 1 || strings.TrimSpace(b.Quote) == "" {
			continue
		}
		valid := false
		for _, name := range RequiredBenefits {
			if b.Name == name {
				valid = true
			}
		}
		if !valid || seen[b.Name] {
			continue
		}
		seen[b.Name] = true
		r.Benefits = append(r.Benefits, b)
	}
	missing := []string{}
	for _, name := range RequiredBenefits {
		if !seen[name] {
			missing = append(missing, name)
		}
	}
	missing = append(missing, info.Missing...)
	if len(missing) > 0 {
		r.warn("sbc_interpretation", "missing_benefits", "Some benefit details could not be supported by a document citation.", "Consult the cited SBC or obtain the complete document.", missing...)
	}
	r.step("sbc_interpretation", "completed")
	r.Status = "ready_with_caution"
	if len(r.Benefits) == 0 {
		r.Status = "partial"
		r.step("sbc_interpretation", "partial")
	}
	r.warn("sbc_interpretation", "ai_interpretation", "Benefits and page citations were extracted by AI and may contain reading errors.", "Check the quoted passages in the saved document before relying on an interpretation.", "independent_interpretation_verification")
	r.warn("member_benefits", "not_verified", "An SBC describes plan terms, not current enrollment, remaining deductible, network participation, or claim approval.", "Confirm current member-specific benefits with the insurer.", "active_coverage", "remaining_deductible", "provider_network", "claim_approval")
}

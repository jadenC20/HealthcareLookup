package billreview

import (
	"clearcare/evidence"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
)

type Line struct {
	ID          string   `json:"line_id"`
	Date        string   `json:"service_date"`
	Description string   `json:"description"`
	Code        string   `json:"code"`
	Modifiers   []string `json:"modifiers,omitempty"`
	Quantity    *int     `json:"quantity"`
	Charge      *float64 `json:"charge"`
	Page        *int     `json:"page,omitempty"`
	Billed      *float64 `json:"billed"`
	Paid        *float64 `json:"plan_paid"`
	Adjustment  *float64 `json:"adjustment"`
	Patient     *float64 `json:"patient_responsibility"`
}
type Bill struct {
	Provider string   `json:"provider_name"`
	Claim    string   `json:"claim_number"`
	Lines    []Line   `json:"lines"`
	Subtotal *float64 `json:"stated_subtotal"`
	Patient  *float64 `json:"patient_responsibility"`
}
type EOB struct {
	Claim   string   `json:"claim_number"`
	Lines   []Line   `json:"lines"`
	Patient *float64 `json:"patient_responsibility_total"`
}
type Request struct {
	Bill    Bill   `json:"bill"`
	EOB     *EOB   `json:"eob"`
	Patient string `json:"patient_name"`
	Account string `json:"account_number"`
}
type Finding struct {
	Rule     string         `json:"rule"`
	Title    string         `json:"title"`
	Message  string         `json:"message"`
	IDs      []string       `json:"line_ids"`
	Evidence map[string]any `json:"evidence"`
	Question string         `json:"question"`
}

func cents(n float64) int64 { return int64(math.Round(n * 100)) }
func knownCents(n *float64) (int64, bool) {
	if n == nil {
		return 0, false
	}
	return cents(*n), true
}
func key(l Line) string {
	service := l.Code
	if service == "" {
		service = strings.ToLower(strings.TrimSpace(l.Description))
	}
	return l.Date + "|" + service
}
func Analyze(r Request) map[string]any {
	findings := []Finding{}
	add := func(rule, title, message string, ids []string, e map[string]any, question string) {
		if ids == nil {
			ids = []string{}
		}
		findings = append(findings, Finding{rule, title, message, ids, e, question})
	}
	groups := map[string][]Line{}
	total, allChargesKnown := int64(0), true
	for _, l := range r.Bill.Lines {
		charge, chargeKnown := knownCents(l.Charge)
		if chargeKnown {
			total += charge
		} else {
			allChargesKnown = false
		}
		if l.Quantity != nil && chargeKnown {
			k := fmt.Sprintf("%s|%s|%s|%d|%d", l.Date, l.Code, strings.ToLower(strings.TrimSpace(l.Description)), *l.Quantity, charge)
			groups[k] = append(groups[k], l)
		}
		desc := strings.ToLower(l.Description)
		if len(strings.TrimSpace(desc)) < 4 || strings.Contains(desc, "fee") || strings.Contains(desc, "service charge") || strings.Contains(desc, "misc") || strings.Contains(desc, "supplies") || strings.Contains(desc, "adjustment") {
			question := fmt.Sprintf("Please explain what %q covers and provide itemized detail.", l.Description)
			if l.Charge != nil {
				question = fmt.Sprintf("Please explain what %q ($%.2f) covers and provide itemized detail.", l.Description, *l.Charge)
			}
			add("unclear_fee", "Charge is not clearly described", "This description is vague. Ask for itemized detail.", []string{l.ID}, map[string]any{"description": l.Description, "charge": l.Charge}, question)
		}
	}
	for _, ls := range groups {
		if len(ls) > 1 {
			ids := []string{}
			for _, l := range ls {
				ids = append(ids, l.ID)
			}
			l := ls[0]
			add("possible_duplicate", "Possible duplicate charge", "These lines have the same date, code, description, quantity, and amount. Please confirm whether they are separate services.", ids, map[string]any{"service_date": l.Date, "code": l.Code, "description": l.Description, "amount_each": l.Charge, "times_listed": len(ls)}, fmt.Sprintf("Line items %s look identical (%s, $%.2f). Were these separate services?", strings.Join(ids, ", "), l.Description, *l.Charge))
		}
	}
	if r.Bill.Subtotal != nil && allChargesKnown && total != cents(*r.Bill.Subtotal) {
		add("subtotal_mismatch", "Line items do not match the subtotal", "The listed amounts do not reconcile with the stated subtotal.", nil, map[string]any{"sum_of_lines": float64(total) / 100, "stated_subtotal": *r.Bill.Subtotal}, "Can you explain the difference between the line items and subtotal?")
	}
	if e := r.EOB; e != nil {
		if r.Bill.Claim != "" && e.Claim != "" && r.Bill.Claim != e.Claim {
			add("claim_number_mismatch", "Bill and EOB have different claim numbers", "These documents may not be for the same claim, so line matching was skipped.", nil, map[string]any{"bill_claim": r.Bill.Claim, "eob_claim": e.Claim}, "Which claim number does this bill belong to?")
		} else {
			bg, eg := map[string][]Line{}, map[string][]Line{}
			for _, l := range r.Bill.Lines {
				bg[key(l)] = append(bg[key(l)], l)
			}
			for _, l := range e.Lines {
				eg[key(l)] = append(eg[key(l)], l)
			}
			for k, ls := range bg {
				if len(e.Lines) == 0 {
					break
				}
				ids := []string{}
				var bt, et int64
				billKnown, eobKnown := true, true
				for _, l := range ls {
					ids = append(ids, l.ID)
					v, ok := knownCents(l.Charge)
					billKnown = billKnown && ok
					bt += v
				}
				es := eg[k]
				for _, l := range es {
					v, ok := knownCents(l.Billed)
					eobKnown = eobKnown && ok
					et += v
				}
				if len(es) == 0 {
					billAmount := any(nil)
					if billKnown {
						billAmount = float64(bt) / 100
					}
					add("bill_line_not_on_eob", "Billed service not found on the EOB", "This service is on the bill but not on the EOB. It may be on a different claim.", ids, map[string]any{"service": k, "bill_amount": billAmount}, "Which claim was this service billed under?")
				} else if billKnown && eobKnown && bt != et {
					add("billed_amount_mismatch", "Bill and EOB show different billed amounts", "The bill total for this service is different from the billed amount on the EOB.", ids, map[string]any{"service": k, "bill_amount": float64(bt) / 100, "eob_billed_amount": float64(et) / 100}, "Why are the billed amounts different?")
				}
			}
			if r.Bill.Patient != nil {
				var ep int64
				known := e.Patient != nil
				if e.Patient != nil {
					ep = cents(*e.Patient)
				} else {
					known = len(e.Lines) > 0
					for _, l := range e.Lines {
						v, ok := knownCents(l.Patient)
						known = known && ok
						ep += v
					}
				}
				if known && cents(*r.Bill.Patient) != ep {
					add("patient_responsibility_mismatch", "Bill and EOB show different patient amounts", "The bill and EOB show different patient-responsibility amounts.", nil, map[string]any{"bill_patient_responsibility": *r.Bill.Patient, "eob_patient_responsibility": float64(ep) / 100}, "Please explain or correct the balance.")
				}
			}
		}
		for _, l := range e.Lines {
			billed, bOK := knownCents(l.Billed)
			adjustment, aOK := knownCents(l.Adjustment)
			paid, pOK := knownCents(l.Paid)
			patient, ptOK := knownCents(l.Patient)
			expected := billed - adjustment - paid
			if bOK && aOK && pOK && ptOK && expected != patient {
				add("eob_line_does_not_add_up", "EOB line does not add up", "Confirm this difference with your insurer.", []string{l.ID}, map[string]any{"expected_patient_amount": float64(expected) / 100, "listed_patient_amount": l.Patient}, "")
			}
		}
	}
	var draft strings.Builder
	fmt.Fprintf(&draft, "Request for itemized bill and clarification\n\nProvider: %s\nClaim: %s\nAccount: %s\n\n", r.Bill.Provider, r.Bill.Claim, r.Account)
	for _, f := range findings {
		if f.Question != "" {
			fmt.Fprintf(&draft, "- %s\n", f.Question)
		}
	}
	fmt.Fprintf(&draft, "\nThank you,\n%s", r.Patient)
	return map[string]any{"flag_count": len(findings), "findings": findings, "inquiry_draft": draft.String(), "note": "Flags are things to confirm, not proof of an error."}
}
func HandleAnalyze(w http.ResponseWriter, r *http.Request) {
	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Bill.Provider) == "" || len(req.Bill.Lines) > 200 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(422)
		json.NewEncoder(w).Encode(map[string]string{"detail": "Invalid bill."})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	evidence.JSON(w, 200, Analyze(req))
}

package account

import (
	"clearcare/billreview"
	"clearcare/caresearch"
	"clearcare/insurance"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type healthcareAI interface {
	ExtractBill(context.Context, insurance.Document) (insurance.BillExtraction, error)
	ReviewBill(context.Context, any, map[string]any, insurance.HealthContext) (insurance.BillAdvice, error)
	ExplainCare(context.Context, []map[string]any, insurance.HealthContext) (insurance.CareAdvice, error)
}

func (s *Service) healthContext(user User) insurance.HealthContext {
	context := insurance.HealthContext{PlanLabel: user.Plan, Status: "no_documented_benefits", Benefits: []insurance.Benefit{}, Cautions: []insurance.Caution{}, Limited: true, Warning: "Your SBC is missing or incomplete. You can continue, but insurance guidance is limited and costs or coverage cannot be established."}
	state, err := s.loadInsurance(user.ID)
	if err != nil {
		return context
	}
	context.Status = state.Result.Status
	context.Details = state.Result.Details
	if state.Result.DetailsConfirmed {
		context.Benefits = state.Result.Benefits
	}
	context.Cautions = state.Result.Cautions
	if state.Result.MayBeInaccurate {
		context.Status = "partial_or_unverified"
	}
	if state.Result.Document != nil {
		context.Source = state.Result.Document.URL
	}
	if state.Result.DetailsConfirmed && len(context.Benefits) > 0 && state.Result.Status == "ready_with_caution" {
		context.Limited = false
		context.Warning = "SBC terms are available, but current enrollment, provider network status and personal deductible balances remain unverified."
	}
	return context
}

func (s *Service) InsuranceContext(w http.ResponseWriter, r *http.Request) {
	u, ok := s.identity(w, r, false)
	if !ok {
		return
	}
	respond(w, 200, s.healthContext(u))
}
func (s *Service) allowHealthcareAI(w http.ResponseWriter, user User) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := "ai:" + user.ID
	now := time.Now()
	kept := []time.Time{}
	for _, t := range s.attempts[key] {
		if now.Sub(t) < 15*time.Minute {
			kept = append(kept, t)
		}
	}
	if len(kept) >= 25 {
		fail(w, 429, "AI request limit reached. Try again in 15 minutes.")
		return false
	}
	s.attempts[key] = append(kept, now)
	return true
}
func (s *Service) Extract(w http.ResponseWriter, r *http.Request) {
	user, ok := s.identity(w, r, true)
	if !ok {
		return
	}
	if err := r.ParseMultipartForm(11 << 20); err != nil {
		fail(w, 400, "Choose a PDF, PNG, or JPEG medical bill.")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	if r.FormValue("consent") != "true" {
		fail(w, 422, "Confirm AI document processing before uploading.")
		return
	}
	if r.MultipartForm == nil || len(r.MultipartForm.File["file"]) != 1 {
		fail(w, 422, "Upload one medical bill.")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		fail(w, 422, "Choose a bill file.")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, insurance.MaxDocumentBytes+1))
	if err != nil || len(data) > insurance.MaxDocumentBytes {
		fail(w, 413, "Bill files must be at most 10 MB.")
		return
	}
	kind := http.DetectContentType(data)
	if kind != "application/pdf" && kind != "image/png" && kind != "image/jpeg" {
		fail(w, 415, "Use a PDF, PNG, or JPEG bill.")
		return
	}
	if !s.allowHealthcareAI(w, user) {
		return
	}
	result, err := s.healthAI.ExtractBill(r.Context(), insurance.NewDocument(data, kind, "uploaded_bill"))
	if err != nil {
		fail(w, 502, err.Error())
		return
	}
	respond(w, 200, map[string]any{"extraction": result, "note": "AI-transcribed details need your confirmation. The original file is not saved to the healthcare database by this upload."})
}
func reviewHash(req billreview.Request) string {
	raw, _ := json.Marshal(req)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func (s *Service) savedAnalysis(uid string, req billreview.Request) map[string]any {
	var raw string
	var result map[string]any
	if s.db.QueryRow(`SELECT payload FROM ai_review_analyses WHERE user_id=? AND request_hash=? AND created>?`, uid, reviewHash(req), time.Now().Add(-24*time.Hour).Unix()).Scan(&raw) == nil && json.Unmarshal([]byte(raw), &result) == nil {
		return result
	}
	return billreview.Analyze(req)
}
func (s *Service) AIReview(w http.ResponseWriter, r *http.Request) {
	user, ok := s.identity(w, r, true)
	if !ok {
		return
	}
	var input struct {
		billreview.Request
		Consent bool `json:"ai_consent"`
	}
	if decode(r, &input) != nil || !input.Consent || strings.TrimSpace(input.Bill.Provider) == "" || len(input.Bill.Lines) == 0 || len(input.Bill.Lines) > 100 {
		fail(w, 422, "Confirm AI processing and enter 1–100 bill lines with a provider.")
		return
	}
	if err := (billreview.ReviewRequest{Request: input.Request, Confirmed: true}).Validate(); err != nil {
		fail(w, 422, err.Error())
		return
	}
	for _, line := range input.Bill.Lines {
		if line.ID == "" || line.Quantity == nil || *line.Quantity < 1 || line.Charge == nil || *line.Charge < 0 {
			fail(w, 422, "Confirm each line ID, quantity, and charge.")
			return
		}
	}
	if !s.allowHealthcareAI(w, user) {
		return
	}
	rules := billreview.Analyze(input.Request)
	plan := s.healthContext(user)
	rules["insurance_context"] = plan
	advice, err := s.healthAI.ReviewBill(r.Context(), input.Request, rules, plan)
	if err != nil {
		rules["ai"] = map[string]any{"status": "unavailable", "detail": err.Error(), "insurance_context": plan}
	} else {
		rules["ai"] = map[string]any{"status": "completed", "result": advice, "insurance_context": plan}
		rules["inquiry_draft"] = advice.Inquiry
	}
	payload, _ := json.Marshal(rules)
	if _, e := s.db.Exec(`INSERT INTO ai_review_analyses(user_id,request_hash,payload,created) VALUES(?,?,?,?) ON CONFLICT(user_id,request_hash) DO UPDATE SET payload=excluded.payload,created=excluded.created`, user.ID, reviewHash(input.Request), string(payload), time.Now().Unix()); e != nil {
		fail(w, 500, "The review finished but could not be retained for saving. Retry.")
		return
	}
	respond(w, 200, rules)
}
func (s *Service) CareRecommendations(care *caresearch.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := s.identity(w, r, false)
		if !ok {
			return
		}
		if r.URL.Query().Get("ai_consent") != "true" {
			fail(w, 422, "Confirm use of your plan information for AI guidance.")
			return
		}
		q := r.URL.Query()
		if q.Get("procedure_code") == "" {
			fail(w, 422, "Enter a procedure code for insurance-aware price guidance.")
			return
		}
		if q.Get("city") == "" {
			q.Set("city", user.City)
		}
		rows, warnings := care.Compare(q)
		filtered := []map[string]any{}
		includeDemo, _ := strconv.ParseBool(q.Get("include_demo"))
		for _, row := range rows {
			if row["is_demo"] == true && !includeDemo {
				continue
			}
			filtered = append(filtered, row)
			if len(filtered) == 40 {
				warnings = append(warnings, "Only the first 40 sourced listings were considered.")
				break
			}
		}
		plan := s.healthContext(user)
		base := map[string]any{"results": filtered, "warnings": warnings, "insurance_context": plan, "coverage_status": "unverified"}
		if len(filtered) == 0 {
			base["ai"] = map[string]any{"status": "no_data", "detail": "No sourced listings are available. AI cannot invent provider prices."}
			respond(w, 200, base)
			return
		}
		if !s.allowHealthcareAI(w, user) {
			return
		}
		advice, err := s.healthAI.ExplainCare(r.Context(), filtered, plan)
		if err != nil {
			base["ai"] = map[string]any{"status": "unavailable", "detail": err.Error()}
		} else {
			base["ai"] = map[string]any{"status": "completed", "result": advice}
		}
		respond(w, 200, base)
	}
}

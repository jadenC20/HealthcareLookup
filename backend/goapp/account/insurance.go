package account

import (
	"bytes"
	"clearcare/insurance"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

type insuranceState struct {
	Revision string           `json:"revision"`
	Result   insurance.Result `json:"result"`
}

func (s *Service) loadInsurance(uid string) (insuranceState, error) {
	var state insuranceState
	var data string
	err := s.db.QueryRow(`SELECT revision,result FROM insurance_workflows WHERE user_id=?`, uid).Scan(&state.Revision, &data)
	if err == nil {
		err = json.Unmarshal([]byte(data), &state.Result)
	}
	return state, err
}
func (s *Service) InsuranceStatus(w http.ResponseWriter, r *http.Request) {
	u, ok := s.identity(w, r, false)
	if !ok {
		return
	}
	state, err := s.loadInsurance(u.ID)
	if err == sql.ErrNoRows {
		result := insurance.NewResult(insurance.Details{}, false)
		result.Status = "not_started"
		result.Finish()
		respond(w, 200, insuranceState{Result: result})
		return
	}
	if err != nil {
		fail(w, 500, "Unable to load insurance lookup.")
		return
	}
	if interrupted(state.Result) {
		state.Result.Status = "interrupted"
		state.Result.Cautions = append(state.Result.Cautions, insurance.Caution{Step: "workflow", Code: "interrupted", Message: "The last lookup did not finish.", Missing: []string{"completed_lookup"}, Action: "Retry the lookup."})
		state.Result.Finish()
	}
	respond(w, 200, state)
}
func interrupted(result insurance.Result) bool {
	updated, _ := time.Parse(time.RFC3339, result.Updated)
	return result.Status == "processing" && time.Since(updated) > 5*time.Minute
}

// A revision guard prevents stale API actions or in-flight external calls from
// overwriting a replacement card, a newer lookup, or a deleted account.
func (s *Service) startInsurance(w http.ResponseWriter, uid, expected string, initial insurance.Result) (string, bool) {
	previous, err := s.loadInsurance(uid)
	if err != nil && err != sql.ErrNoRows {
		fail(w, 500, "Unable to start insurance lookup.")
		return "", false
	}
	if err == nil && (expected == "" || previous.Revision != expected) {
		fail(w, 409, "Insurance information changed. Reload it and try again.")
		return "", false
	}
	if err == nil && previous.Result.Status == "processing" && !interrupted(previous.Result) {
		fail(w, 409, "An insurance lookup is already running.")
		return "", false
	}
	if err == sql.ErrNoRows && expected != "" {
		fail(w, 409, "Insurance information was removed. Reload it and try again.")
		return "", false
	}
	initial.Status = "processing"
	initial.Finish()
	payload, _ := json.Marshal(initial)
	revision := token()
	var res sql.Result
	if err == sql.ErrNoRows {
		res, err = s.db.Exec(`INSERT INTO insurance_workflows(user_id,revision,result,document) SELECT ?,?,?,NULL WHERE EXISTS(SELECT 1 FROM users WHERE id=?) ON CONFLICT(user_id) DO NOTHING`, uid, revision, string(payload), uid)
	} else {
		res, err = s.db.Exec(`UPDATE insurance_workflows SET revision=?,result=?,document=NULL WHERE user_id=? AND revision=?`, revision, string(payload), uid, expected)
	}
	if err != nil {
		fail(w, 500, "Unable to start insurance lookup.")
		return "", false
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		fail(w, 409, "Insurance information changed. Reload it and try again.")
		return "", false
	}
	return revision, true
}
func (s *Service) runInsurance(w http.ResponseWriter, r *http.Request, uid, revision string, run func(context.Context) insurance.Result) {
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Minute)
	defer cancel()
	result := run(ctx)
	result.Finish()
	payload, err := json.Marshal(result)
	if err != nil {
		fail(w, 500, "Unable to save insurance results.")
		return
	}
	var document []byte
	if result.Document != nil {
		document = result.Document.Data
	}
	res, err := s.db.Exec(`UPDATE insurance_workflows SET result=?,document=? WHERE user_id=? AND revision=? AND EXISTS(SELECT 1 FROM users WHERE id=?)`, string(payload), document, uid, revision, uid)
	if err != nil {
		fail(w, 500, "Unable to save insurance results.")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		fail(w, 409, "The card or account changed during lookup. Reload the profile.")
		return
	}
	respond(w, 200, insuranceState{Revision: revision, Result: result})
}
func decodeInsurance(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 32<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (s *Service) ConfirmInsurance(w http.ResponseWriter, r *http.Request) {
	u, ok := s.identity(w, r, true)
	if !ok {
		return
	}
	var body struct {
		Revision string            `json:"revision"`
		Details  insurance.Details `json:"details"`
	}
	if decodeInsurance(r, &body) != nil {
		fail(w, 422, "Provide revision and plan details.")
		return
	}
	if err := body.Details.Validate(); err != nil {
		fail(w, 422, err.Error())
		return
	}
	if body.Details.Insurer == "" || (body.Details.PlanName == "" && body.Details.PlanID == "") || body.Details.State == "" || body.Details.Year == 0 {
		fail(w, 422, "Confirm the insurer, plan name or public plan ID, state and coverage year before SBC discovery.")
		return
	}
	previous, _ := s.loadInsurance(u.ID)
	var saved *insurance.Document
	if previous.Revision == body.Revision && previous.Result.Document != nil {
		doc := *previous.Result.Document
		if err := s.db.QueryRow(`SELECT document FROM insurance_workflows WHERE user_id=? AND revision=?`, u.ID, body.Revision).Scan(&doc.Data); err == nil && len(doc.Data) > 0 {
			saved = &doc
		}
	}
	revision, ok := s.startInsurance(w, u.ID, body.Revision, insurance.NewResult(body.Details, true))
	if !ok {
		return
	}
	s.runInsurance(w, r, u.ID, revision, func(ctx context.Context) insurance.Result {
		if saved != nil {
			confirmed := insurance.NewResult(body.Details, true)
			confirmed.Selected = previous.Result.Selected
			return s.insurance.Reinterpret(ctx, confirmed, *saved)
		}
		return s.insurance.Run(ctx, insurance.Document{}, &body.Details)
	})
}
func (s *Service) RetryInsurance(w http.ResponseWriter, r *http.Request) {
	u, ok := s.identity(w, r, true)
	if !ok {
		return
	}
	var body struct {
		Revision string `json:"revision"`
	}
	if decodeInsurance(r, &body) != nil {
		fail(w, 422, "Provide the current revision.")
		return
	}
	state, err := s.loadInsurance(u.ID)
	if err != nil {
		fail(w, 404, "Start with a card upload or plan details.")
		return
	}
	var card insurance.Document
	var saved *insurance.Document
	if state.Result.Document != nil {
		doc := *state.Result.Document
		err = s.db.QueryRow(`SELECT document FROM insurance_workflows WHERE user_id=? AND revision=?`, u.ID, state.Revision).Scan(&doc.Data)
		if err != nil || len(doc.Data) == 0 {
			fail(w, 409, "The saved SBC changed. Reload insurance information.")
			return
		}
		saved = &doc
	}
	if saved == nil && !state.Result.DetailsConfirmed {
		err = s.db.QueryRow(`SELECT data,content_type FROM insurance_cards WHERE user_id=?`, u.ID).Scan(&card.Data, &card.MIME)
		if err != nil {
			fail(w, 422, "Upload a card or confirm plan details before retrying.")
			return
		}
	}
	initial := insurance.NewResult(state.Result.Details, state.Result.DetailsConfirmed)
	revision, ok := s.startInsurance(w, u.ID, body.Revision, initial)
	if !ok {
		return
	}
	s.runInsurance(w, r, u.ID, revision, func(ctx context.Context) insurance.Result {
		if saved != nil {
			return s.insurance.Reinterpret(ctx, state.Result, *saved)
		}
		var d *insurance.Details
		if state.Result.DetailsConfirmed {
			d = &state.Result.Details
		}
		return s.insurance.Run(ctx, card, d)
	})
}
func (s *Service) SelectInsurance(w http.ResponseWriter, r *http.Request) {
	u, ok := s.identity(w, r, true)
	if !ok {
		return
	}
	var body struct {
		Revision    string `json:"revision"`
		CandidateID string `json:"candidate_id"`
	}
	if decodeInsurance(r, &body) != nil {
		fail(w, 422, "Provide revision and candidate_id.")
		return
	}
	state, err := s.loadInsurance(u.ID)
	if err != nil {
		fail(w, 404, "Run plan lookup first.")
		return
	}
	found := false
	for _, c := range state.Result.Candidates {
		if c.ID == body.CandidateID {
			found = true
		}
	}
	if !found {
		fail(w, 422, "Select a candidate from the current lookup.")
		return
	}
	revision, ok := s.startInsurance(w, u.ID, body.Revision, insurance.NewResult(state.Result.Details, state.Result.DetailsConfirmed))
	if !ok {
		return
	}
	s.runInsurance(w, r, u.ID, revision, func(ctx context.Context) insurance.Result {
		result, _ := s.insurance.Select(ctx, state.Result, body.CandidateID)
		return result
	})
}
func (s *Service) UploadSBC(w http.ResponseWriter, r *http.Request) {
	u, ok := s.identity(w, r, true)
	if !ok {
		return
	}
	reader, err := r.MultipartReader()
	if err != nil {
		fail(w, 400, "Upload an SBC PDF and the current revision.")
		return
	}
	var data []byte
	revision := ""
	fileSeen := false
	for {
		part, e := reader.NextPart()
		if e == io.EOF {
			break
		}
		if e != nil {
			fail(w, 400, "Invalid multipart upload.")
			return
		}
		switch part.FormName() {
		case "file":
			if fileSeen {
				part.Close()
				fail(w, 422, "Upload one SBC file.")
				return
			}
			fileSeen = true
			data, err = io.ReadAll(io.LimitReader(part, insurance.MaxDocumentBytes+1))
		case "revision":
			var b []byte
			b, err = io.ReadAll(io.LimitReader(part, 256))
			revision = string(b)
		default:
			err = io.ErrUnexpectedEOF
		}
		part.Close()
		if err != nil {
			fail(w, 400, "Invalid SBC upload fields.")
			return
		}
	}
	if len(data) > insurance.MaxDocumentBytes {
		fail(w, 413, "SBC must be 10 MB or smaller.")
		return
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		fail(w, 415, "Upload an SBC PDF.")
		return
	}
	state, err := s.loadInsurance(u.ID)
	if err == sql.ErrNoRows {
		state.Result = insurance.NewResult(insurance.Details{}, false)
	} else if err != nil {
		fail(w, 500, "Unable to load insurance lookup.")
		return
	}
	next, ok := s.startInsurance(w, u.ID, revision, insurance.NewResult(state.Result.Details, state.Result.DetailsConfirmed))
	if !ok {
		return
	}
	s.runInsurance(w, r, u.ID, next, func(ctx context.Context) insurance.Result {
		return s.insurance.Upload(ctx, state.Result, insurance.NewDocument(data, "application/pdf", "user_upload"))
	})
}
func (s *Service) SBCFile(w http.ResponseWriter, r *http.Request) {
	u, ok := s.identity(w, r, false)
	if !ok {
		return
	}
	var data []byte
	err := s.db.QueryRow(`SELECT document FROM insurance_workflows WHERE user_id=?`, u.ID).Scan(&data)
	if err == sql.ErrNoRows || err == nil && len(data) == 0 {
		fail(w, 404, "No SBC document is saved.")
		return
	}
	if err != nil {
		fail(w, 500, "Unable to load SBC.")
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="plan-sbc.pdf"`)
	w.Write(data)
}
func (s *Service) DeleteInsurance(w http.ResponseWriter, r *http.Request) {
	u, ok := s.identity(w, r, true)
	if !ok {
		return
	}
	if _, err := s.db.Exec(`DELETE FROM insurance_workflows WHERE user_id=?`, u.ID); err != nil {
		fail(w, 500, "Unable to delete insurance results.")
		return
	}
	respond(w, 200, map[string]bool{"ok": true})
}

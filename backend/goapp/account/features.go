package account

import (
	"bytes"
	"clearcare/billreview"
	"clearcare/caresearch"
	"clearcare/evidence"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"time"
)

type bufferedResponse struct {
	Code    int
	Body    bytes.Buffer
	headers http.Header
}

func (b *bufferedResponse) Header() http.Header { return b.headers }
func (b *bufferedResponse) WriteHeader(status int) {
	if b.Code == 0 {
		b.Code = status
	}
}
func (b *bufferedResponse) Write(data []byte) (int, error) {
	if b.Code == 0 {
		b.Code = 200
	}
	return b.Body.Write(data)
}

// Feature protects expensive operations and persists only successful structured results.
func (s *Service) Feature(feature string, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := s.identity(w, r, r.Method != "GET")
		if !ok {
			return
		}
		if !s.allowFeature(w, u.ID) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 4*time.Minute)
		defer cancel()
		r = r.WithContext(ctx)
		rec := &bufferedResponse{headers: make(http.Header)}
		handler(rec, r)
		if rec.Code != 200 {
			for k, values := range rec.Header() {
				w.Header()[k] = values
			}
			w.WriteHeader(rec.Code)
			w.Write(rec.Body.Bytes())
			return
		}
		var data any
		if json.Unmarshal(rec.Body.Bytes(), &data) != nil {
			fail(w, 500, "Unable to save feature result.")
			return
		}
		id := token()
		res, err := s.db.Exec(`INSERT INTO feature_runs(id,user_id,feature,created,result) SELECT ?,?,?,?,? WHERE EXISTS(SELECT 1 FROM users WHERE id=?)`, id, u.ID, feature, time.Now().UTC().Format(time.RFC3339), rec.Body.String(), u.ID)
		if err != nil {
			fail(w, 500, "Unable to save feature result.")
			return
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			fail(w, 409, "Account changed during processing.")
			return
		}
		respond(w, 200, map[string]any{"id": id, "feature": feature, "result": data})
	}
}
func (s *Service) Preferences(w http.ResponseWriter, r *http.Request) {
	u, ok := s.identity(w, r, r.Method != "GET")
	if !ok {
		return
	}
	if r.Method == "GET" {
		var data string
		err := s.db.QueryRow(`SELECT payload FROM patient_preferences WHERE user_id=?`, u.ID).Scan(&data)
		if err == sql.ErrNoRows {
			respond(w, 200, caresearch.Preferences{})
			return
		}
		if err != nil {
			fail(w, 500, "Unable to load preferences.")
			return
		}
		respond(w, 200, json.RawMessage(data))
		return
	}
	var p caresearch.Preferences
	if evidence.Decode(r, &p) != nil || !p.Validate() {
		fail(w, 422, "Provide specialty, city, state and valid patient preferences.")
		return
	}
	data, _ := json.Marshal(p)
	res, err := s.db.Exec(`INSERT INTO patient_preferences(user_id,payload) SELECT ?,? WHERE EXISTS(SELECT 1 FROM users WHERE id=?) ON CONFLICT(user_id) DO UPDATE SET payload=excluded.payload`, u.ID, string(data), u.ID)
	if err != nil {
		fail(w, 500, "Unable to save preferences.")
		return
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		fail(w, 409, "Account changed.")
		return
	}
	respond(w, 200, p)
}
func (s *Service) Recommend(care *caresearch.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Explicit request values override saved preferences; {} uses the saved profile.
		u, ok := s.identity(w, r, true)
		if !ok {
			return
		}
		if !s.allowFeature(w, u.ID) {
			return
		}
		var raw map[string]json.RawMessage
		if evidence.Decode(r, &raw) != nil {
			fail(w, 422, "Invalid preferences.")
			return
		}
		p := caresearch.Preferences{}
		var data string
		err := s.db.QueryRow(`SELECT payload FROM patient_preferences WHERE user_id=?`, u.ID).Scan(&data)
		if err == nil {
			json.Unmarshal([]byte(data), &p)
		} else if err != sql.ErrNoRows {
			fail(w, 500, "Unable to load preferences.")
			return
		}
		merged, _ := json.Marshal(p)
		var fields map[string]json.RawMessage
		json.Unmarshal(merged, &fields)
		for k, v := range raw {
			if _, exists := fields[k]; !exists {
				fail(w, 422, "Unknown preference field.")
				return
			}
			fields[k] = v
		}
		merged, _ = json.Marshal(fields)
		if json.Unmarshal(merged, &p) != nil || !p.Validate() {
			fail(w, 422, "Provide specialty, city, state and valid preferences.")
			return
		}
		if p.InsurancePlan == "" {
			if state, err := s.loadInsurance(u.ID); err == nil && state.Result.DetailsConfirmed {
				p.InsurancePlan = state.Result.Details.Insurer + " " + state.Result.Details.PlanName
			}
		}
		evidence.JSON(w, 200, care.Recommend(r.Context(), p))
	}
}
func (s *Service) Runs(w http.ResponseWriter, r *http.Request) {
	u, ok := s.identity(w, r, r.Method == "DELETE")
	if !ok {
		return
	}
	id := r.PathValue("id")
	if r.Method == "DELETE" {
		tx, err := s.db.Begin()
		if err != nil {
			fail(w, 500, "Unable to delete result.")
			return
		}
		defer tx.Rollback()
		_, err = tx.Exec(`DELETE FROM charge_observations WHERE id IN (SELECT observation_id FROM observation_owners WHERE user_id=? AND run_id=?)`, u.ID, id)
		if err == nil {
			_, err = tx.Exec(`DELETE FROM observation_owners WHERE user_id=? AND run_id=?`, u.ID, id)
		}
		var deleted int64
		if err == nil {
			var res sql.Result
			res, err = tx.Exec(`DELETE FROM feature_runs WHERE id=? AND user_id=?`, id, u.ID)
			if err == nil {
				deleted, _ = res.RowsAffected()
			}
		}
		if err != nil || tx.Commit() != nil {
			fail(w, 500, "Unable to delete result.")
			return
		}
		if deleted == 0 {
			fail(w, 404, "Result not found.")
			return
		}
		respond(w, 200, map[string]any{"id": id, "deleted": true})
		return
	}
	if id != "" {
		var feature, created, result string
		err := s.db.QueryRow(`SELECT feature,created,result FROM feature_runs WHERE id=? AND user_id=?`, id, u.ID).Scan(&feature, &created, &result)
		if err == sql.ErrNoRows {
			fail(w, 404, "Result not found.")
			return
		}
		if err != nil {
			fail(w, 500, "Unable to load result.")
			return
		}
		respond(w, 200, map[string]any{"id": id, "feature": feature, "created": created, "result": json.RawMessage(result)})
		return
	}
	limit, cursor := pageLimit(r), r.URL.Query().Get("cursor")
	created, cursorID := decodeCursor(cursor)
	feature := r.URL.Query().Get("feature")
	query := `SELECT id,feature,created FROM feature_runs WHERE user_id=?`
	args := []any{u.ID}
	if feature != "" {
		query += ` AND feature=?`
		args = append(args, feature)
	}
	if created != "" {
		query += ` AND (created<? OR (created=? AND id<?))`
		args = append(args, created, created, cursorID)
	}
	query += ` ORDER BY created DESC,id DESC LIMIT ?`
	args = append(args, limit+1)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		fail(w, 500, "Unable to list results.")
		return
	}
	defer rows.Close()
	out := []map[string]string{}
	for rows.Next() {
		var id, feature, created string
		if rows.Scan(&id, &feature, &created) != nil {
			fail(w, 500, "Unable to read results.")
			return
		}
		out = append(out, map[string]string{"id": id, "feature": feature, "created": created})
	}
	next := ""
	if len(out) > limit {
		last := out[limit-1]
		next = encodeCursor(last["created"], last["id"])
		out = out[:limit]
	}
	respond(w, 200, map[string]any{"results": out, "next_cursor": next})
}
func (s *Service) ReviewBill(w http.ResponseWriter, r *http.Request) {
	u, ok := s.identity(w, r, true)
	if !ok {
		return
	}
	if !s.allowFeature(w, u.ID) {
		return
	}
	var req billreview.ReviewRequest
	if evidence.Decode(r, &req) != nil {
		fail(w, 422, "Invalid bill request.")
		return
	}
	if err := req.Validate(); err != nil {
		fail(w, 422, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Minute)
	defer cancel()
	dir := os.Getenv("CLEARCARE_DATA_DIR")
	if dir == "" {
		dir = "../data"
	}
	result := billreview.Review(ctx, dir, req)
	if r.Context().Err() != nil {
		fail(w, 408, "Bill review was canceled or timed out.")
		return
	}
	data, err := json.Marshal(result)
	if err != nil {
		fail(w, 500, "Unable to serialize result.")
		return
	}
	tx, err := s.db.Begin()
	if err != nil {
		fail(w, 500, "Unable to save result.")
		return
	}
	defer tx.Rollback()
	var exists int
	if tx.QueryRow(`SELECT 1 FROM users WHERE id=?`, u.ID).Scan(&exists) != nil {
		fail(w, 409, "Account changed during processing.")
		return
	}
	id := token()
	_, err = tx.Exec(`INSERT INTO feature_runs VALUES(?,?,?,?,?)`, id, u.ID, "bill_review", time.Now().UTC().Format(time.RFC3339), string(data))
	// Observation rows deliberately exclude user, claim, service date and document text.
	if req.FacilityID != "" && result.FacilityVerified {
		for _, c := range result.Comparisons {
			if c.Reference.Origin == "public_search" {
				continue
			} // Unverified web rates remain private review evidence, not observations.
			if err != nil {
				break
			}
			oid := token()
			_, err = tx.Exec(`INSERT INTO charge_observations VALUES(?,?,?,?,?,?,?,?)`, oid, req.FacilityID, c.Code, c.Charge, c.BenchmarkTotal, c.Difference, c.Reference.PriceType, c.Reference.Source)
			if err == nil {
				_, err = tx.Exec(`INSERT INTO observation_owners VALUES(?,?,?)`, oid, u.ID, id)
			}
		}
	}
	if err != nil || tx.Commit() != nil {
		fail(w, 500, "Unable to save result.")
		return
	}
	respond(w, 200, map[string]any{"id": id, "feature": "bill_review", "result": result})
}
func (s *Service) Observations(w http.ResponseWriter, r *http.Request) {
	u, ok := s.identity(w, r, false)
	if !ok {
		return
	}
	limit := pageLimit(r)
	cursor := r.URL.Query().Get("cursor")
	query := `SELECT c.id,c.facility_id,c.code,c.charge,c.benchmark,c.difference,c.price_type,c.source_url FROM charge_observations c JOIN observation_owners o ON c.id=o.observation_id WHERE o.user_id=?`
	args := []any{u.ID}
	if cursor != "" {
		query += ` AND c.id<?`
		args = append(args, cursor)
	}
	query += ` ORDER BY c.id DESC LIMIT ?`
	args = append(args, limit+1)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		fail(w, 500, "Unable to load observations.")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, facility, code, typ, source string
		var charge, benchmark, diff float64
		if rows.Scan(&id, &facility, &code, &charge, &benchmark, &diff, &typ, &source) != nil {
			fail(w, 500, "Unable to read observations.")
			return
		}
		out = append(out, map[string]any{"id": id, "facility_id": facility, "code": code, "charge": charge, "benchmark": benchmark, "over_under_benchmark": diff, "price_type": typ, "source_url": source})
	}
	next := ""
	if len(out) > limit {
		next, _ = out[limit-1]["id"].(string)
		out = out[:limit]
	}
	respond(w, 200, map[string]any{"results": out, "next_cursor": next})
}

func (s *Service) allowFeature(w http.ResponseWriter, uid string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := "feature:" + uid
	now := time.Now()
	kept := []time.Time{}
	for _, t := range s.attempts[key] {
		if now.Sub(t) < time.Minute {
			kept = append(kept, t)
		}
	}
	if len(kept) >= 6 {
		s.attempts[key] = kept
		w.Header().Set("Retry-After", "60")
		fail(w, 429, "Too many automated requests. Retry in a minute.")
		return false
	}
	s.attempts[key] = append(kept, now)
	return true
}

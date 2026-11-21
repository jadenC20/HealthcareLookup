package account

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type jobView struct {
	ID        string `json:"id"`
	Feature   string `json:"feature"`
	Status    string `json:"status"`
	Progress  int    `json:"progress"`
	Stage     string `json:"stage"`
	Created   string `json:"created"`
	Updated   string `json:"updated"`
	ResultID  string `json:"result_id,omitempty"`
	Result    any    `json:"result,omitempty"`
	Error     string `json:"error,omitempty"`
	PollURL   string `json:"poll_url"`
	CancelURL string `json:"cancel_url"`
}

func (s *Service) Protected(mutate bool, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.identity(w, r, mutate); !ok {
			return
		}
		handler(w, r)
	}
}

// AsyncFeature persists job state while keeping uploaded/request bodies in memory only.
func (s *Service) AsyncFeature(feature string, handler http.HandlerFunc, handlerPersists, rateLimit bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := s.identity(w, r, true)
		if !ok {
			return
		}
		if rateLimit && !s.allowFeature(w, u.ID) {
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			fail(w, 422, "Unable to read request.")
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		digest := sha256.Sum256(append([]byte(r.Method+"\n"+r.URL.RequestURI()+"\n"+r.Header.Get("Content-Type")+"\n"), body...))
		requestHash := hex.EncodeToString(digest[:])
		var existing string
		err = s.db.QueryRow(`SELECT id FROM feature_jobs WHERE user_id=? AND feature=? AND request_hash=? AND status IN ('queued','running')`, u.ID, feature, requestHash).Scan(&existing)
		if err == nil {
			respond(w, 202, map[string]any{"job": s.loadJob(u.ID, existing), "duplicate": true})
			return
		}
		if err != sql.ErrNoRows {
			fail(w, 500, "Unable to create job.")
			return
		}
		id, now := token(), time.Now().UTC().Format(time.RFC3339)
		_, err = s.db.Exec(`INSERT INTO feature_jobs(id,user_id,feature,request_hash,status,progress,stage,created,updated) VALUES(?,?,?,?,'queued',10,'queued',?,?)`, id, u.ID, feature, requestHash, now, now)
		if err != nil {
			if s.db.QueryRow(`SELECT id FROM feature_jobs WHERE user_id=? AND feature=? AND request_hash=? AND status IN ('queued','running')`, u.ID, feature, requestHash).Scan(&existing) == nil {
				respond(w, 202, map[string]any{"job": s.loadJob(u.ID, existing), "duplicate": true})
				return
			}
			fail(w, 500, "Unable to create job.")
			return
		}
		background, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		s.jobsMu.Lock()
		s.jobCancels[id] = cancel
		s.jobsMu.Unlock()
		request := r.Clone(background)
		request.Body = io.NopCloser(bytes.NewReader(body))
		s.jobWG.Add(1)
		go s.runJob(background, cancel, id, u.ID, feature, request, handler, handlerPersists)
		respond(w, 202, map[string]any{"job": s.loadJob(u.ID, id), "duplicate": false})
	}
}

func (s *Service) runJob(ctx context.Context, cancel context.CancelFunc, id, uid, feature string, r *http.Request, handler http.HandlerFunc, handlerPersists bool) {
	defer s.jobWG.Done()
	defer cancel()
	defer func() { s.jobsMu.Lock(); delete(s.jobCancels, id); s.jobsMu.Unlock() }()
	select {
	case s.jobSem <- struct{}{}:
		defer func() { <-s.jobSem }()
	case <-ctx.Done():
		s.finishCanceled(id, ctx.Err())
		return
	}
	s.updateJob(id, "running", 25, "processing", "")
	rec := &bufferedResponse{headers: make(http.Header)}
	handler(rec, r)
	if ctx.Err() != nil {
		s.finishCanceled(id, ctx.Err())
		return
	}
	if rec.Code < 200 || rec.Code >= 300 {
		s.updateJob(id, "failed", 100, "failed", responseError(rec))
		return
	}
	s.updateJob(id, "running", 85, "saving_result", "")
	var result any
	if json.Unmarshal(rec.Body.Bytes(), &result) != nil {
		s.updateJob(id, "failed", 100, "failed", "Feature returned invalid JSON.")
		return
	}
	resultID := ""
	if envelope, ok := result.(map[string]any); ok && handlerPersists {
		resultID, _ = envelope["id"].(string)
		if product, exists := envelope["result"]; exists {
			result = product
		}
	}
	if !handlerPersists {
		resultID = token()
		if _, err := s.db.Exec(`INSERT INTO feature_runs(id,user_id,feature,created,result) SELECT ?,?,?,?,? WHERE EXISTS(SELECT 1 FROM users WHERE id=?)`, resultID, uid, feature, time.Now().UTC().Format(time.RFC3339), rec.Body.String(), uid); err != nil {
			s.updateJob(id, "failed", 100, "failed", "Unable to save feature result.")
			return
		}
	}
	encoded, _ := json.Marshal(result)
	_, err := s.db.Exec(`UPDATE feature_jobs SET status='completed',progress=100,stage='completed',updated=?,result_id=?,result=?,error='' WHERE id=? AND cancel_requested=0`, time.Now().UTC().Format(time.RFC3339), resultID, string(encoded), id)
	if err != nil {
		s.updateJob(id, "failed", 100, "failed", "Unable to complete job.")
	}
}
func responseError(rec *bufferedResponse) string {
	var body struct {
		Detail string `json:"detail"`
	}
	if json.Unmarshal(rec.Body.Bytes(), &body) == nil && body.Detail != "" {
		return body.Detail
	}
	return fmt.Sprintf("Feature request failed with status %d.", rec.Code)
}
func (s *Service) updateJob(id, status string, progress int, stage, message string) {
	_, _ = s.db.Exec(`UPDATE feature_jobs SET status=?,progress=?,stage=?,error=?,updated=? WHERE id=? AND status NOT IN ('completed','canceled')`, status, progress, stage, message, time.Now().UTC().Format(time.RFC3339), id)
}
func (s *Service) finishCanceled(id string, err error) {
	message := "Job canceled."
	if err == context.DeadlineExceeded {
		message = "Job timed out."
	}
	s.updateJob(id, "canceled", 100, "canceled", message)
}
func (s *Service) loadJob(uid, id string) jobView {
	var j jobView
	var raw string
	err := s.db.QueryRow(`SELECT id,feature,status,progress,stage,created,updated,result_id,result,error FROM feature_jobs WHERE id=? AND user_id=?`, id, uid).Scan(&j.ID, &j.Feature, &j.Status, &j.Progress, &j.Stage, &j.Created, &j.Updated, &j.ResultID, &raw, &j.Error)
	if err != nil {
		return jobView{}
	}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &j.Result)
	}
	j.PollURL = "/jobs/" + j.ID
	j.CancelURL = "/jobs/" + j.ID
	return j
}

func (s *Service) Jobs(w http.ResponseWriter, r *http.Request) {
	u, ok := s.identity(w, r, r.Method == "DELETE")
	if !ok {
		return
	}
	id := r.PathValue("id")
	if id != "" && r.Method == "DELETE" {
		res, err := s.db.Exec(`UPDATE feature_jobs SET cancel_requested=1,status='canceled',progress=100,stage='canceled',error='Job canceled.',updated=? WHERE id=? AND user_id=? AND status IN ('queued','running')`, time.Now().UTC().Format(time.RFC3339), id, u.ID)
		if err != nil {
			fail(w, 500, "Unable to cancel job.")
			return
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			fail(w, 409, "Job is not active or does not exist.")
			return
		}
		s.jobsMu.Lock()
		cancel := s.jobCancels[id]
		s.jobsMu.Unlock()
		if cancel != nil {
			cancel()
		}
		respond(w, 200, map[string]any{"job": s.loadJob(u.ID, id)})
		return
	}
	if id != "" {
		j := s.loadJob(u.ID, id)
		if j.ID == "" {
			fail(w, 404, "Job not found.")
			return
		}
		respond(w, 200, map[string]any{"job": j})
		return
	}
	limit, cursor := pageLimit(r), r.URL.Query().Get("cursor")
	created, cursorID := decodeCursor(cursor)
	query := `SELECT id FROM feature_jobs WHERE user_id=?`
	args := []any{u.ID}
	if created != "" {
		query += ` AND (created<? OR (created=? AND id<?))`
		args = append(args, created, created, cursorID)
	}
	query += ` ORDER BY created DESC,id DESC LIMIT ?`
	args = append(args, limit+1)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		fail(w, 500, "Unable to list jobs.")
		return
	}
	ids := []string{}
	for rows.Next() {
		var jid string
		if rows.Scan(&jid) != nil {
			rows.Close()
			fail(w, 500, "Unable to read jobs.")
			return
		}
		ids = append(ids, jid)
	}
	if err := rows.Close(); err != nil {
		fail(w, 500, "Unable to read jobs.")
		return
	}
	jobs := make([]jobView, 0, len(ids))
	for _, jid := range ids {
		jobs = append(jobs, s.loadJob(u.ID, jid))
	}
	next := ""
	if len(jobs) > limit {
		last := jobs[limit-1]
		next = encodeCursor(last.Created, last.ID)
		jobs = jobs[:limit]
	}
	respond(w, 200, map[string]any{"jobs": jobs, "next_cursor": next})
}
func pageLimit(r *http.Request) int {
	n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if n < 1 {
		return 25
	}
	if n > 100 {
		return 100
	}
	return n
}
func encodeCursor(created, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(created + "\n" + id))
}
func decodeCursor(v string) (string, string) {
	b, err := base64.RawURLEncoding.DecodeString(v)
	if err != nil {
		return "", ""
	}
	parts := strings.SplitN(string(b), "\n", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

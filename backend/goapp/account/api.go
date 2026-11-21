package account

import (
	"bytes"
	"clearcare/billreview"
	"clearcare/evidence"
	"clearcare/insurance"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/jung-kurt/gofpdf"
	"golang.org/x/crypto/scrypt"
	"io"
	"net/http"
	"net/mail"
	"os"
	"strings"
	"time"
)

type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
	City  string `json:"city"`
	Plan  string `json:"plan"`
}
type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}
type review struct {
	ID      string          `json:"id"`
	Title   string          `json:"title"`
	Created string          `json:"created"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

func (s *Service) Close() error {
	s.jobsMu.Lock()
	for _, cancel := range s.jobCancels {
		cancel()
	}
	s.jobsMu.Unlock()
	s.jobWG.Wait()
	return s.db.Close()
}
func token() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func hashToken(v string) string { h := sha256.Sum256([]byte(v)); return hex.EncodeToString(h[:]) }
func password(p, salt string) (string, error) {
	raw, e := hex.DecodeString(salt)
	if e != nil {
		return "", e
	}
	h, e := scrypt.Key([]byte(p), raw, 16384, 8, 1, 64)
	if e != nil {
		return "", e
	}
	return salt + ":" + hex.EncodeToString(h), nil
}
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(evidence.Present(v))
}
func fail(w http.ResponseWriter, status int, msg string) {
	respond(w, status, map[string]string{"detail": msg})
}
func decode(r *http.Request, v any) error { return json.NewDecoder(r.Body).Decode(v) }
func (s *Service) throttle(r *http.Request) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := strings.Split(r.RemoteAddr, ":")[0]
	now := time.Now()
	kept := []time.Time{}
	for _, t := range s.attempts[key] {
		if now.Sub(t) < 15*time.Minute {
			kept = append(kept, t)
		}
	}
	if len(kept) >= 25 {
		s.attempts[key] = kept
		return false
	}
	s.attempts[key] = append(kept, now)
	return true
}
func (s *Service) issue(w http.ResponseWriter, uid string) (string, error) {
	raw, csrf := token(), token()
	_, e := s.db.Exec(`DELETE FROM sessions WHERE expires < ?`, time.Now().Unix())
	if e != nil {
		return "", e
	}
	_, e = s.db.Exec(`INSERT INTO sessions VALUES (?,?,?,?)`, hashToken(raw), uid, csrf, time.Now().Add(12*time.Hour).Unix())
	if e != nil {
		return "", e
	}
	http.SetCookie(w, &http.Cookie{Name: "clearcare_session", Value: raw, Path: "/", HttpOnly: true, Secure: os.Getenv("CLEARCARE_SECURE_COOKIES") == "1", SameSite: http.SameSiteStrictMode, MaxAge: 43200})
	return csrf, nil
}
func (s *Service) identity(w http.ResponseWriter, r *http.Request, mutate bool) (User, bool) {
	if r.Header.Get("Authorization") != "" {
		return s.supabaseIdentity(w, r)
	}
	var u User
	cookie, e := r.Cookie("clearcare_session")
	if e != nil {
		fail(w, 401, "Please sign in first.")
		return u, false
	}
	var csrf string
	e = s.db.QueryRow(`SELECT u.id,u.email,u.name,u.city,u.plan,s.csrf FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token=? AND s.expires>?`, hashToken(cookie.Value), time.Now().Unix()).Scan(&u.ID, &u.Email, &u.Name, &u.City, &u.Plan, &csrf)
	if e != nil {
		fail(w, 401, "Please sign in first.")
		return u, false
	}
	if mutate && subtle.ConstantTimeCompare([]byte(csrf), []byte(r.Header.Get("X-CSRF-Token"))) != 1 {
		fail(w, 403, "Session verification failed. Refresh and try again.")
		return u, false
	}
	return u, true
}
func (s *Service) Register(w http.ResponseWriter, r *http.Request) {
	if !s.throttle(r) {
		fail(w, 429, "Too many attempts. Try again in 15 minutes.")
		return
	}
	var c credentials
	if decode(r, &c) != nil {
		fail(w, 422, "Invalid input.")
		return
	}
	c.Email = strings.ToLower(strings.TrimSpace(c.Email))
	c.Name = strings.TrimSpace(c.Name)
	addr, e := mail.ParseAddress(c.Email)
	if e != nil || addr.Address != c.Email || !strings.Contains(c.Email, ".") || len(c.Email) > 254 || len(c.Name) == 0 || len(c.Name) > 100 || len(c.Password) < 12 || len(c.Password) > 128 {
		fail(w, 422, "Enter a valid email, display name, and password of 12 to 128 characters.")
		return
	}
	salt := token()[:32]
	p, e := password(c.Password, salt)
	if e != nil {
		fail(w, 500, "Unable to register.")
		return
	}
	u := User{ID: token()[:32], Email: c.Email, Name: c.Name}
	_, e = s.db.Exec(`INSERT INTO users(id,email,name,password) VALUES(?,?,?,?)`, u.ID, u.Email, u.Name, p)
	if e != nil {
		fail(w, 409, "Unable to register with this email. Try signing in.")
		return
	}
	csrf, e := s.issue(w, u.ID)
	if e != nil {
		fail(w, 500, "Unable to create session.")
		return
	}
	respond(w, 200, map[string]any{"user": u, "csrf": csrf})
}
func (s *Service) Login(w http.ResponseWriter, r *http.Request) {
	if !s.throttle(r) {
		fail(w, 429, "Too many attempts. Try again in 15 minutes.")
		return
	}
	var c credentials
	if decode(r, &c) != nil || len(c.Password) > 128 {
		fail(w, 422, "Invalid input.")
		return
	}
	var u User
	var stored string
	e := s.db.QueryRow(`SELECT id,email,name,city,plan,password FROM users WHERE email=?`, strings.ToLower(strings.TrimSpace(c.Email))).Scan(&u.ID, &u.Email, &u.Name, &u.City, &u.Plan, &stored)
	if e != nil {
		stored = "00000000000000000000000000000000:" + strings.Repeat("0", 128)
	}
	parts := strings.Split(stored, ":")
	if len(parts) != 2 {
		fail(w, 401, "Email or password is incorrect.")
		return
	}
	hashed, err := password(c.Password, parts[0])
	if err != nil || subtle.ConstantTimeCompare([]byte(hashed), []byte(stored)) != 1 || e != nil {
		fail(w, 401, "Email or password is incorrect.")
		return
	}
	if old, err := r.Cookie("clearcare_session"); err == nil {
		s.db.Exec(`DELETE FROM sessions WHERE token=?`, hashToken(old.Value))
	}
	csrf, err := s.issue(w, u.ID)
	if err != nil {
		fail(w, 500, "Unable to create session.")
		return
	}
	respond(w, 200, map[string]any{"user": u, "csrf": csrf})
}
func (s *Service) Me(w http.ResponseWriter, r *http.Request) {
	u, ok := s.identity(w, r, false)
	if !ok {
		return
	}
	if r.Header.Get("Authorization") != "" {
		respond(w, 200, map[string]any{"user": u, "csrf": ""})
		return
	}
	c, _ := r.Cookie("clearcare_session")
	var csrf string
	if s.db.QueryRow(`SELECT csrf FROM sessions WHERE token=?`, hashToken(c.Value)).Scan(&csrf) != nil {
		fail(w, 401, "Please sign in first.")
		return
	}
	respond(w, 200, map[string]any{"user": u, "csrf": csrf})
}
func (s *Service) Logout(w http.ResponseWriter, r *http.Request) {
	_, ok := s.identity(w, r, true)
	if !ok {
		return
	}
	if r.Header.Get("Authorization") != "" {
		respond(w, 200, map[string]bool{"ok": true})
		return
	}
	c, _ := r.Cookie("clearcare_session")
	s.db.Exec(`DELETE FROM sessions WHERE token=?`, hashToken(c.Value))
	http.SetCookie(w, &http.Cookie{Name: "clearcare_session", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	respond(w, 200, map[string]bool{"ok": true})
}
func (s *Service) Profile(w http.ResponseWriter, r *http.Request) {
	u, ok := s.identity(w, r, true)
	if !ok {
		return
	}
	var p User
	if decode(r, &p) != nil || len(strings.TrimSpace(p.Name)) == 0 || len(p.Name) > 100 || len(p.City) > 100 || len(p.Plan) > 200 {
		fail(w, 422, "Invalid profile.")
		return
	}
	_, e := s.db.Exec(`UPDATE users SET name=?,city=?,plan=? WHERE id=?`, p.Name, p.City, p.Plan, u.ID)
	if e != nil {
		fail(w, 500, "Unable to update profile.")
		return
	}
	u.Name, u.City, u.Plan = p.Name, p.City, p.Plan
	respond(w, 200, map[string]any{"user": u})
}
func (s *Service) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	u, ok := s.identity(w, r, true)
	if !ok {
		return
	}
	if err := s.deletePrivateDocuments(r.Context(), u.ID, ""); err != nil {
		fail(w, 503, "Unable to delete private files. Please retry.")
		return
	}
	tx, e := s.db.Begin()
	if e != nil {
		fail(w, 500, "Unable to delete account.")
		return
	}
	defer tx.Rollback()
	for _, query := range []string{`DELETE FROM charge_observations WHERE id IN (SELECT observation_id FROM observation_owners WHERE user_id=?)`, `DELETE FROM observation_owners WHERE user_id=?`, `DELETE FROM feature_jobs WHERE user_id=?`, `DELETE FROM feature_runs WHERE user_id=?`, `DELETE FROM patient_preferences WHERE user_id=?`, `DELETE FROM ai_review_analyses WHERE user_id=?`, `DELETE FROM insurance_workflows WHERE user_id=?`, `DELETE FROM insurance_cards WHERE user_id=?`, `DELETE FROM reviews WHERE user_id=?`, `DELETE FROM sessions WHERE user_id=?`, `DELETE FROM users WHERE id=?`} {
		if _, e = tx.Exec(query, u.ID); e != nil {
			fail(w, 500, "Unable to delete account.")
			return
		}
	}
	if tx.Commit() != nil {
		fail(w, 500, "Unable to delete account.")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "clearcare_session", Path: "/", MaxAge: -1, HttpOnly: true})
	respond(w, 200, map[string]bool{"ok": true})
}
func (s *Service) Save(w http.ResponseWriter, r *http.Request) {
	u, ok := s.identity(w, r, true)
	if !ok {
		return
	}
	var req struct {
		billreview.Request
		SaveAI bool `json:"save_ai"`
	}
	if decode(r, &req) != nil || strings.TrimSpace(req.Bill.Provider) == "" || len(req.Bill.Lines) == 0 || len(req.Bill.Lines) > 200 {
		fail(w, 422, "Add at least one reviewed bill line.")
		return
	}
	analysis := billreview.Analyze(req.Request)
	if req.SaveAI {
		analysis = s.savedAnalysis(u.ID, req.Request)
	}
	payload, e := json.Marshal(map[string]any{"request": req.Request, "analysis": analysis})
	if e != nil {
		fail(w, 422, "Invalid review.")
		return
	}
	v := review{ID: token()[:32], Title: req.Bill.Provider, Created: time.Now().UTC().Format(time.RFC3339Nano), Payload: payload}
	_, e = s.db.Exec(`INSERT INTO reviews VALUES (?,?,?,?,?)`, v.ID, u.ID, v.Title, v.Created, string(payload))
	if e != nil {
		fail(w, 500, "Unable to save review.")
		return
	}
	respond(w, 200, v)
}
func (s *Service) List(w http.ResponseWriter, r *http.Request) {
	u, ok := s.identity(w, r, false)
	if !ok {
		return
	}
	rows, e := s.db.Query(`SELECT id,title,created FROM reviews WHERE user_id=? ORDER BY created DESC`, u.ID)
	if e != nil {
		fail(w, 500, "Unable to list reviews.")
		return
	}
	defer rows.Close()
	list := []review{}
	for rows.Next() {
		var v review
		if rows.Scan(&v.ID, &v.Title, &v.Created) == nil {
			list = append(list, v)
		}
	}
	respond(w, 200, map[string]any{"results": list})
}
func (s *Service) owned(uid, user string) (review, error) {
	var v review
	var payload string
	e := s.db.QueryRow(`SELECT id,title,created,payload FROM reviews WHERE id=? AND user_id=?`, uid, user).Scan(&v.ID, &v.Title, &v.Created, &payload)
	v.Payload = json.RawMessage(payload)
	return v, e
}
func (s *Service) Get(w http.ResponseWriter, r *http.Request) {
	u, ok := s.identity(w, r, false)
	if !ok {
		return
	}
	v, e := s.owned(r.PathValue("id"), u.ID)
	if e != nil {
		fail(w, 404, "Review not found.")
		return
	}
	respond(w, 200, v)
}
func (s *Service) Delete(w http.ResponseWriter, r *http.Request) {
	u, ok := s.identity(w, r, true)
	if !ok {
		return
	}
	res, e := s.db.Exec(`DELETE FROM reviews WHERE id=? AND user_id=?`, r.PathValue("id"), u.ID)
	if e != nil {
		fail(w, 500, "Unable to delete review.")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		fail(w, 404, "Review not found.")
		return
	}
	respond(w, 200, map[string]bool{"ok": true})
}
func pdf(w http.ResponseWriter, records []review, user *User) {
	p := gofpdf.New("P", "mm", "A4", "")
	p.AddPage()
	p.SetFont("Arial", "B", 16)
	p.CellFormat(0, 10, "ClearCare | Healthcare cost review", "", 1, "", false, 0, "")
	p.SetFont("Arial", "", 10)
	p.MultiCell(0, 6, "Review flags are questions to confirm, not proof of errors.", "", "", false)
	if user != nil {
		p.MultiCell(0, 6, fmt.Sprintf("Profile: %s | %s | %s | %s", user.Name, user.Email, user.City, user.Plan), "", "", false)
	}
	for _, record := range records {
		var payload struct {
			Request  billreview.Request `json:"request"`
			Analysis struct {
				Findings []billreview.Finding `json:"findings"`
				Inquiry  string               `json:"inquiry_draft"`
				AI       struct {
					Status string               `json:"status"`
					Detail string               `json:"detail"`
					Result insurance.BillAdvice `json:"result"`
				} `json:"ai"`
			} `json:"analysis"`
		}
		if json.Unmarshal(record.Payload, &payload) != nil {
			continue
		}
		p.SetFont("Arial", "B", 13)
		p.MultiCell(0, 8, record.Title, "", "", false)
		p.SetFont("Arial", "", 10)
		p.MultiCell(0, 6, fmt.Sprintf("Claim: %s\n", payload.Request.Bill.Claim), "", "", false)
		for _, l := range payload.Request.Bill.Lines {
			p.MultiCell(0, 6, fmt.Sprintf("%s | %s | %s | %s | quantity %s | charge %s | page %v", l.ID, l.Date, l.Description, l.Code, optionalInt(l.Quantity), optionalMoney(l.Charge), pageNumber(l.Page)), "", "", false)
			if l.Quote != "" {
				p.MultiCell(0, 6, "AI-transcribed source quote for "+l.ID+": "+l.Quote, "", "", false)
			}
		}
		if eob := payload.Request.EOB; eob != nil {
			p.MultiCell(0, 6, fmt.Sprintf("EOB claim: %s", eob.Claim), "", "", false)
			if eob.Patient != nil {
				p.MultiCell(0, 6, fmt.Sprintf("EOB patient responsibility: $%.2f", *eob.Patient), "", "", false)
			}
			for _, l := range eob.Lines {
				p.MultiCell(0, 6, fmt.Sprintf("%s | %s | %s | billed %s | paid %s | adjustment %s | patient %s", l.ID, l.Date, l.Description, optionalMoney(l.Billed), optionalMoney(l.Paid), optionalMoney(l.Adjustment), optionalMoney(l.Patient)), "", "", false)
			}
		}
		if payload.Analysis.AI.Status == "completed" {
			p.MultiCell(0, 6, "AI explanation: "+payload.Analysis.AI.Result.Summary, "", "", false)
			for _, item := range payload.Analysis.AI.Result.Items {
				p.MultiCell(0, 6, "Lines "+strings.Join(item.Lines, ", ")+": "+item.Explanation+" Question: "+item.Question, "", "", false)
			}
			for _, note := range payload.Analysis.AI.Result.Limitations {
				p.MultiCell(0, 6, "AI limitation: "+note, "", "", false)
			}
		}
		for _, f := range payload.Analysis.Findings {
			p.MultiCell(0, 6, f.Title+": "+f.Message, "", "", false)
			evidence, _ := json.Marshal(f.Evidence)
			p.MultiCell(0, 6, "Lines: "+strings.Join(f.IDs, ", ")+" | Evidence: "+string(evidence), "", "", false)
		}
		p.MultiCell(0, 6, "Billing inquiry:\n"+payload.Analysis.Inquiry, "", "", false)
	}
	var buf bytes.Buffer
	if p.Output(&buf) != nil {
		fail(w, 500, "Unable to generate PDF.")
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="clearcare-report.pdf"`)
	w.Write(buf.Bytes())
}
func pageNumber(page *int) any {
	if page == nil {
		return "?"
	}
	return *page
}
func optionalInt(v *int) string {
	if v == nil {
		return "unknown"
	}
	return fmt.Sprintf("%d", *v)
}
func optionalMoney(v *float64) string {
	if v == nil {
		return "unknown"
	}
	return fmt.Sprintf("$%.2f", *v)
}
func (s *Service) SavedPDF(w http.ResponseWriter, r *http.Request) {
	u, ok := s.identity(w, r, false)
	if !ok {
		return
	}
	v, e := s.owned(r.PathValue("id"), u.ID)
	if e != nil {
		fail(w, 404, "Review not found.")
		return
	}
	pdf(w, []review{v}, nil)
}
func (s *Service) CurrentPDF(w http.ResponseWriter, r *http.Request) {
	var req struct {
		billreview.Request
		SaveAI bool `json:"save_ai"`
	}
	if decode(r, &req) != nil || strings.TrimSpace(req.Bill.Provider) == "" {
		fail(w, 422, "Invalid review.")
		return
	}
	analysis := billreview.Analyze(req.Request)
	if req.SaveAI {
		u, ok := s.identity(w, r, false)
		if !ok {
			return
		}
		analysis = s.savedAnalysis(u.ID, req.Request)
	}
	b, _ := json.Marshal(map[string]any{"request": req.Request, "analysis": analysis})
	pdf(w, []review{{Title: req.Bill.Provider, Payload: b}}, nil)
}
func (s *Service) AccountPDF(w http.ResponseWriter, r *http.Request) {
	u, ok := s.identity(w, r, false)
	if !ok {
		return
	}
	rows, e := s.db.Query(`SELECT id,title,created,payload FROM reviews WHERE user_id=? ORDER BY created`, u.ID)
	if e != nil {
		fail(w, 500, "Unable to export account.")
		return
	}
	defer rows.Close()
	records := []review{}
	for rows.Next() {
		var v review
		var b string
		if rows.Scan(&v.ID, &v.Title, &v.Created, &b) == nil {
			v.Payload = json.RawMessage(b)
			records = append(records, v)
		}
	}
	pdf(w, records, &u)
}

// UploadCard stores the card and starts extraction and SBC discovery immediately.
func (s *Service) UploadCard(w http.ResponseWriter, r *http.Request) {
	u, ok := s.identity(w, r, true)
	if !ok {
		return
	}
	reader, err := r.MultipartReader()
	if err != nil {
		fail(w, 400, "Choose a card file.")
		return
	}
	part, err := reader.NextPart()
	if err != nil || part.FormName() != "file" {
		fail(w, 400, "Choose a card file.")
		return
	}
	defer part.Close()
	data, err := io.ReadAll(io.LimitReader(part, 10*1024*1024+1))
	if err != nil {
		fail(w, 400, "Unable to read the card.")
		return
	}
	if len(data) > 10*1024*1024 {
		fail(w, 413, "Card must be 10 MB or smaller.")
		return
	}
	kind := http.DetectContentType(data)
	if kind != "image/png" && kind != "image/jpeg" && kind != "application/pdf" {
		fail(w, 415, "Use a PNG, JPEG, or PDF card file.")
		return
	}
	updated := time.Now().UTC().Format(time.RFC3339Nano)
	revision := token()
	initial := insurance.NewResult(insurance.Details{}, false)
	initial.Status = "processing"
	initial.Finish()
	payload, _ := json.Marshal(initial)
	stored, err := s.storeDocument(r.Context(), u.ID, "card", kind, data)
	if err != nil {
		fail(w, 503, "Unable to save private card file.")
		return
	}
	tx, err := s.db.Begin()
	if err != nil {
		fail(w, 500, "Unable to save the card.")
		return
	}
	defer tx.Rollback()
	var accountExists int
	if err = tx.QueryRow(`SELECT 1 FROM users WHERE id=?`, u.ID).Scan(&accountExists); err != nil {
		fail(w, 409, "The account changed during upload. Sign in again.")
		return
	}
	_, err = tx.Exec(`INSERT INTO insurance_cards(user_id,content_type,data,updated) VALUES(?,?,?,?) ON CONFLICT(user_id) DO UPDATE SET content_type=excluded.content_type,data=excluded.data,updated=excluded.updated`, u.ID, kind, stored, updated)
	if err == nil {
		_, err = tx.Exec(`INSERT INTO insurance_workflows(user_id,revision,result,document) VALUES(?,?,?,NULL) ON CONFLICT(user_id) DO UPDATE SET revision=excluded.revision,result=excluded.result,document=NULL`, u.ID, revision, string(payload))
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		fail(w, 500, "Unable to save the card.")
		return
	}
	s.runInsurance(w, r, u.ID, revision, func(ctx context.Context) insurance.Result {
		return s.insurance.Run(ctx, insurance.NewDocument(data, kind, "uploaded_card"), nil)
	})
}

func (s *Service) CardStatus(w http.ResponseWriter, r *http.Request) {
	u, ok := s.identity(w, r, false)
	if !ok {
		return
	}
	var updated string
	err := s.db.QueryRow(`SELECT updated FROM insurance_cards WHERE user_id=?`, u.ID).Scan(&updated)
	if err == sql.ErrNoRows {
		respond(w, 200, map[string]any{"has_card": false, "lookup_status": "not_available"})
		return
	}
	if err != nil {
		fail(w, 500, "Unable to load card status.")
		return
	}
	state, err := s.loadInsurance(u.ID)
	if err != nil && err != sql.ErrNoRows {
		fail(w, 500, "Unable to load insurance status.")
		return
	}
	status := "not_started"
	if err == nil {
		status = state.Result.Status
	}
	respond(w, 200, map[string]any{"has_card": true, "updated": updated, "lookup_status": status})
}

func (s *Service) DeleteCard(w http.ResponseWriter, r *http.Request) {
	u, ok := s.identity(w, r, true)
	if !ok {
		return
	}
	if err := s.deletePrivateDocuments(r.Context(), u.ID, ""); err != nil {
		fail(w, 503, "Unable to delete private files. Please retry.")
		return
	}
	tx, err := s.db.Begin()
	if err != nil {
		fail(w, 500, "Unable to delete the card.")
		return
	}
	defer tx.Rollback()
	_, err = tx.Exec(`DELETE FROM insurance_workflows WHERE user_id=?`, u.ID)
	if err == nil {
		_, err = tx.Exec(`DELETE FROM insurance_cards WHERE user_id=?`, u.ID)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		fail(w, 500, "Unable to delete the card.")
		return
	}
	respond(w, 200, map[string]bool{"ok": true})
}

func (s *Service) CardFile(w http.ResponseWriter, r *http.Request) {
	u, ok := s.identity(w, r, false)
	if !ok {
		return
	}
	var data []byte
	var kind string
	err := s.db.QueryRow(`SELECT content_type,data FROM insurance_cards WHERE user_id=?`, u.ID).Scan(&kind, &data)
	if err == nil {
		data, err = s.readDocument(r.Context(), u.ID, data)
	}
	if err == sql.ErrNoRows {
		fail(w, 404, "Card not found.")
		return
	}
	if err != nil {
		fail(w, 500, "Unable to load the card.")
		return
	}
	w.Header().Set("Content-Type", kind)
	w.Header().Set("Content-Disposition", `attachment; filename="insurance-card"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write(data)
}

// Package insurance implements card-to-SBC discovery independently of HTTP and storage.
package insurance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const MaxDocumentBytes = 10 << 20
const CautionMessage = "This may be inaccurate because parts of the automated process are missing or could not be verified."

type Details struct {
	Insurer  string `json:"insurer"`
	PlanName string `json:"plan_name"`
	PlanID   string `json:"plan_id"`
	Network  string `json:"network"`
	State    string `json:"state"`
	Year     int    `json:"year"`
	Employer string `json:"employer"`
}

func (d Details) Validate() error {
	for _, s := range []string{d.Insurer, d.PlanName, d.PlanID, d.Network, d.State, d.Employer} {
		if len(s) > 300 {
			return errors.New("Plan fields must be at most 300 characters.")
		}
	}
	if d.Year != 0 && (d.Year < 2014 || d.Year > time.Now().Year()+2) {
		return errors.New("Enter a valid coverage year, or 0 if unknown.")
	}
	if d.State != "" && (len(d.State) != 2 || strings.ToUpper(d.State) != d.State || d.State[0] < 'A' || d.State[0] > 'Z' || d.State[1] < 'A' || d.State[1] > 'Z') {
		return errors.New("State must be a two-letter uppercase abbreviation.")
	}
	return nil
}

type Extraction struct {
	Details    Details  `json:"details"`
	Unreadable []string `json:"unreadable_fields"`
}
type Candidate struct {
	ID        string  `json:"id"`
	Details   Details `json:"details"`
	URL       string  `json:"url"`
	Source    string  `json:"source"`
	SourceURL string  `json:"source_url"`
	Match     string  `json:"match"`
}

func (c *Candidate) Identify() {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%d|%s", c.URL, c.Details.PlanID, c.Details.Year, c.Details.State)))
	c.ID = hex.EncodeToString(h[:12])
}

type Benefit struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Quote string `json:"quote"`
	Page  int    `json:"page"`
}
type Interpretation struct {
	IsSBC    bool      `json:"is_sbc"`
	Details  Details   `json:"details"`
	Benefits []Benefit `json:"benefits"`
	Missing  []string  `json:"missing_fields"`
}
type Document struct {
	Data        []byte `json:"-"`
	MIME        string `json:"content_type"`
	URL         string `json:"source_url"`
	SHA256      string `json:"sha256"`
	RetrievedAt string `json:"retrieved_at"`
}

func NewDocument(data []byte, mime, url string) Document {
	h := sha256.Sum256(data)
	return Document{Data: data, MIME: mime, URL: url, SHA256: hex.EncodeToString(h[:]), RetrievedAt: time.Now().UTC().Format(time.RFC3339)}
}

type Step struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}
type Caution struct {
	Step    string   `json:"step"`
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Missing []string `json:"missing"`
	Action  string   `json:"action"`
}
type Result struct {
	Status           string      `json:"status"`
	MayBeInaccurate  bool        `json:"may_be_inaccurate"`
	Caution          string      `json:"caution"`
	Details          Details     `json:"details"`
	DetailsConfirmed bool        `json:"details_confirmed"`
	Candidates       []Candidate `json:"candidates"`
	Selected         *Candidate  `json:"selected,omitempty"`
	Document         *Document   `json:"document,omitempty"`
	DocumentDetails  *Details    `json:"document_details,omitempty"`
	Benefits         []Benefit   `json:"benefits"`
	Steps            []Step      `json:"steps"`
	Cautions         []Caution   `json:"cautions"`
	Updated          string      `json:"updated"`
}

func NewResult(d Details, confirmed bool) Result {
	return Result{Status: "partial", Details: d, DetailsConfirmed: confirmed, Candidates: []Candidate{}, Benefits: []Benefit{}, Steps: []Step{}, Cautions: []Caution{}}
}
func (r *Result) step(name, status string) {
	for i := range r.Steps {
		if r.Steps[i].Name == name {
			r.Steps[i].Status = status
			return
		}
	}
	r.Steps = append(r.Steps, Step{name, status})
}
func (r *Result) warn(step, code, msg, action string, missing ...string) {
	if missing == nil {
		missing = []string{}
	}
	r.Cautions = append(r.Cautions, Caution{step, code, msg, missing, action})
}
func (r *Result) Finish() {
	r.Updated = time.Now().UTC().Format(time.RFC3339)
	r.MayBeInaccurate = len(r.Cautions) > 0
	if r.MayBeInaccurate {
		r.Caution = CautionMessage
	} else {
		r.Caution = ""
	}
}

type Provider interface {
	Extract(context.Context, Document) (Extraction, error)
	Discover(context.Context, Details) ([]Candidate, error)
	Interpret(context.Context, Document) (Interpretation, error)
}
type Catalog interface {
	Find(context.Context, Details) ([]Candidate, error)
}
type Fetcher interface {
	Fetch(context.Context, string) (Document, error)
}
type Engine struct {
	AI      Provider
	Catalog Catalog
	Fetcher Fetcher
}

type StageError struct {
	Code    string
	Message string
}

func (e *StageError) Error() string     { return e.Code + ": " + e.Message }
func stageError(code, msg string) error { return &StageError{code, msg} }
func (r *Result) failure(stage string, err error) {
	code, msg := "unavailable", "The service could not finish this step."
	var e *StageError
	if errors.As(err, &e) {
		code, msg = e.Code, e.Message
	}
	r.step(stage, "failed")
	r.warn(stage, code, msg, "Retry this step, correct the plan details, or upload the SBC manually.", stage)
}

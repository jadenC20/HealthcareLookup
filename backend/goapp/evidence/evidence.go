// Package evidence supplies common uncertainty, source, and API boundaries.
package evidence

import (
	"clearcare/insurance"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

type Report struct {
	MayBeInaccurate bool                `json:"may_be_inaccurate"`
	Caution         string              `json:"caution"`
	Cautions        []insurance.Caution `json:"cautions"`
}

func New() Report { return Report{Cautions: []insurance.Caution{}} }
func (r *Report) Warn(step, code, message, action string, missing ...string) {
	r.MayBeInaccurate = true
	r.Caution = insurance.CautionMessage
	r.Cautions = append(r.Cautions, insurance.Caution{Step: step, Code: code, Message: message, Action: action, Missing: missing})
}
func (r *Report) Failure(step string, err error) {
	code, message := "unavailable", "This step could not be completed."
	var e *insurance.StageError
	if errors.As(err, &e) {
		code, message = e.Code, e.Message
	}
	r.Warn(step, code, message, "Retry or supply verified reference data.", step)
}
func JSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(Present(value))
}
func Decode(r *http.Request, value any) error {
	d := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("unexpected trailing input")
	}
	return nil
}
func Object(fields map[string]any) map[string]any {
	req := []string{}
	for k := range fields {
		req = append(req, k)
	}
	return map[string]any{"type": "object", "properties": fields, "required": req, "additionalProperties": false}
}
func String() any         { return map[string]any{"type": "string"} }
func Number() any         { return map[string]any{"type": "number"} }
func Array(item any) any  { return map[string]any{"type": "array", "items": item} }
func NullableNumber() any { return map[string]any{"type": []string{"number", "null"}} }
func SourceAllowed(url string, sources []string) bool {
	if insurance.SafeURL(url) != nil {
		return false
	}
	for _, s := range sources {
		if s == url {
			return true
		}
	}
	return false
}
func PublicJSON(ctx context.Context, url string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ClearCare/1.0")
	client := &http.Client{Timeout: 25 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error { return errors.New("unexpected redirect") }}
	res, err := client.Do(req)
	if err != nil {
		return errors.New("public API unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return errors.New("public API rejected request")
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, (4<<20)+1))
	if err != nil || len(b) > 4<<20 {
		return errors.New("public API response too large or incomplete")
	}
	return json.Unmarshal(b, dest)
}
func Clean(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

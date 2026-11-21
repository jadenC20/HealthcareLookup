package account

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

const objectPrefix = "clearcare-private-object:"

type privateDocuments struct {
	origin, key, bucket string
	client              *http.Client
}

func newPrivateDocuments() (*privateDocuments, error) {
	if os.Getenv("CLEARCARE_DOCUMENT_STORE") != "supabase" {
		return nil, nil
	}
	origin := strings.TrimRight(os.Getenv("SUPABASE_URL"), "/")
	u, err := url.Parse(origin)
	key, bucket := os.Getenv("SUPABASE_SERVICE_ROLE_KEY"), os.Getenv("SUPABASE_STORAGE_BUCKET")
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || key == "" || !regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString(bucket) {
		return nil, fmt.Errorf("private Supabase document storage is not configured")
	}
	d := &privateDocuments{origin: origin, key: key, bucket: bucket, client: &http.Client{Timeout: 45 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	data, err := d.request(ctx, "GET", "/bucket/"+bucket, nil, "")
	var info struct {
		Public bool   `json:"public"`
		ID     string `json:"id"`
	}
	if err != nil || json.Unmarshal(data, &info) != nil || info.ID != bucket || info.Public {
		return nil, fmt.Errorf("storage bucket must exist and be private")
	}
	return d, nil
}
func (d *privateDocuments) request(ctx context.Context, method, path string, data []byte, mime string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, d.origin+"/storage/v1"+path, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("apikey", d.key)
	req.Header.Set("Authorization", "Bearer "+d.key)
	if mime != "" {
		req.Header.Set("Content-Type", mime)
	}
	res, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("private storage unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("private storage request failed (%d)", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 10<<20+1))
	if err != nil || len(body) > 10<<20 {
		return nil, fmt.Errorf("private document exceeds limit")
	}
	return body, nil
}

// Immutable keys plus revision-guarded DB references prevent stale uploads
// overwriting newer documents. The journal also tracks failed/stale uploads.
func (s *Service) storeDocument(ctx context.Context, uid, kind, mime string, data []byte) ([]byte, error) {
	if s.documents == nil || len(data) == 0 {
		return data, nil
	}
	key := hashToken(uid) + "/" + kind + "/" + token()
	if _, err := s.db.Exec(`INSERT INTO private_documents(object_key,user_id,kind,created) SELECT ?,?,?,? WHERE EXISTS(SELECT 1 FROM users WHERE id=?)`, key, uid, kind, time.Now().UTC().Format(time.RFC3339), uid); err != nil {
		return nil, err
	}
	if _, err := s.documents.request(ctx, "POST", "/object/"+s.documents.bucket+"/"+key, data, mime); err != nil {
		return nil, err
	}
	return []byte(objectPrefix + key), nil
}
func (s *Service) readDocument(ctx context.Context, uid string, data []byte) ([]byte, error) {
	if !bytes.HasPrefix(data, []byte(objectPrefix)) {
		return data, nil
	}
	if s.documents == nil {
		return nil, fmt.Errorf("private storage is not configured")
	}
	key := string(bytes.TrimPrefix(data, []byte(objectPrefix)))
	var exists int
	if err := s.db.QueryRow(`SELECT 1 FROM private_documents WHERE object_key=? AND user_id=?`, key, uid).Scan(&exists); err != nil {
		return nil, fmt.Errorf("document unavailable")
	}
	return s.documents.request(ctx, "GET", "/object/authenticated/"+s.documents.bucket+"/"+key, nil, "")
}
func (s *Service) deletePrivateDocuments(ctx context.Context, uid, kind string) error {
	if s.documents == nil {
		return nil
	}
	q := `SELECT object_key FROM private_documents WHERE user_id=?`
	args := []any{uid}
	if kind != "" {
		q += ` AND kind=?`
		args = append(args, kind)
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return err
	}
	keys := []string{}
	for rows.Next() {
		var key string
		if err = rows.Scan(&key); err != nil {
			rows.Close()
			return err
		}
		keys = append(keys, key)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, key := range keys {
		payload, _ := json.Marshal(map[string]any{"prefixes": []string{key}})
		if _, err = s.documents.request(ctx, "DELETE", "/object/"+s.documents.bucket, payload, "application/json"); err != nil {
			return err
		}
		if _, err = s.db.Exec(`DELETE FROM private_documents WHERE object_key=? AND user_id=?`, key, uid); err != nil {
			return err
		}
	}
	return nil
}

package account

import (
	"encoding/json"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

type supabaseVerifier struct {
	origin, key string
	client      *http.Client
}

type supabaseUser struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	Confirmed string `json:"email_confirmed_at"`
	Anonymous bool   `json:"is_anonymous"`
	Metadata  struct {
		Name string `json:"display_name"`
	} `json:"user_metadata"`
}

func newSupabaseVerifier() *supabaseVerifier {
	origin := strings.TrimRight(os.Getenv("SUPABASE_URL"), "/")
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost"))) {
		origin = ""
	}
	key := os.Getenv("SUPABASE_PUBLISHABLE_KEY")
	if key == "" {
		key = os.Getenv("SUPABASE_ANON_KEY")
	}
	return &supabaseVerifier{origin: origin, key: key, client: &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

// Verify against the configured project's Auth server. Never trust a decoded
// token, a browser-supplied user ID, email, or user_metadata for authorization.
func (s *Service) supabaseIdentity(w http.ResponseWriter, r *http.Request) (User, bool) {
	var user User
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || len(parts[1]) > 16384 {
		fail(w, 401, "Sign in with Supabase again.")
		return user, false
	}
	v := s.supabase
	if v == nil || v.origin == "" || v.key == "" {
		fail(w, 503, "Supabase sign-in is not configured on the healthcare backend. Start it with SUPABASE_URL and SUPABASE_PUBLISHABLE_KEY.")
		return user, false
	}
	req, err := http.NewRequestWithContext(r.Context(), "GET", v.origin+"/auth/v1/user", nil)
	if err != nil {
		fail(w, 503, "Unable to verify sign-in.")
		return user, false
	}
	req.Header.Set("Authorization", "Bearer "+parts[1])
	req.Header.Set("apikey", v.key)
	response, err := v.client.Do(req)
	if err != nil {
		fail(w, 503, "Supabase sign-in verification is temporarily unavailable. Retry shortly.")
		return user, false
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		if response.StatusCode == 401 || response.StatusCode == 403 {
			fail(w, 401, "Your Supabase session is no longer valid. Sign in again.")
		} else {
			fail(w, 503, "Supabase sign-in verification is temporarily unavailable.")
		}
		return user, false
	}
	var verified supabaseUser
	if json.NewDecoder(io.LimitReader(response.Body, 256<<10)).Decode(&verified) != nil {
		fail(w, 503, "Unable to verify sign-in.")
		return user, false
	}
	id, err := uuid.Parse(verified.ID)
	address, emailErr := mail.ParseAddress(verified.Email)
	if err != nil || id == uuid.Nil || emailErr != nil || address.Address != verified.Email || len(verified.Email) > 254 || verified.Role != "authenticated" || verified.Anonymous || verified.Confirmed == "" {
		fail(w, 401, "Use a confirmed Supabase email account to access healthcare records.")
		return user, false
	}
	// A namespaced key isolates Supabase identities from legacy accounts. Never
	// automatically merge existing medical records just because emails match.
	user.ID = "supabase:" + id.String()
	name := strings.TrimSpace(verified.Metadata.Name)
	if name == "" || len(name) > 100 {
		name = strings.Split(verified.Email, "@")[0]
	}
	_, err = s.db.Exec(`INSERT INTO users(id,email,name,password) VALUES(?,?,?,?) ON CONFLICT(id) DO NOTHING`, user.ID, user.ID, name, "!supabase-only")
	if err != nil {
		fail(w, 500, "Unable to open your healthcare profile.")
		return User{}, false
	}
	err = s.db.QueryRow(`SELECT name,city,plan FROM users WHERE id=?`, user.ID).Scan(&user.Name, &user.City, &user.Plan)
	if err != nil {
		fail(w, 500, "Unable to load your healthcare profile.")
		return User{}, false
	}
	user.Email = verified.Email
	return user, true
}

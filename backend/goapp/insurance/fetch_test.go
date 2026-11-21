package insurance

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestUnsafeURLs(t *testing.T) {
	for _, url := range []string{"http://insurer.example/sbc.pdf", "https://127.0.0.1/a", "https://[::1]/a", "https://10.0.0.1/a", "https://169.254.169.254/a", "https://user:pass@example.org/a", "https://example.org:8000/a", "https://localhost/a"} {
		if SafeURL(url) == nil {
			t.Errorf("accepted %s", url)
		}
	}
	for _, ip := range []string{"127.0.0.1", "10.0.0.1", "100.100.100.100", "::1", "fe80::1"} {
		if publicIP(net.ParseIP(ip)) {
			t.Errorf("accepted %s", ip)
		}
	}
}
func TestFetchRejectsHTMLAndOversize(t *testing.T) {
	for _, body := range []string{"<html>Sign in</html>", "%PDF-" + strings.Repeat("x", MaxDocumentBytes)} {
		f := &HTTPFetcher{client: &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
		})}}
		if _, err := f.Fetch(context.Background(), "https://insurer.example/sbc.pdf"); err == nil {
			t.Fatal("invalid download accepted")
		}
	}
}

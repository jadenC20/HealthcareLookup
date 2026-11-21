package insurance

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func SafeURL(raw string) error {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || (u.Port() != "" && u.Port() != "443") || strings.HasSuffix(strings.ToLower(u.Hostname()), ".local") || strings.EqualFold(u.Hostname(), "localhost") {
		return stageError("unsafe_url", "The source URL is not a supported public HTTPS document URL.")
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && !publicIP(ip) {
		return stageError("unsafe_url", "Private network document URLs are not allowed.")
	}
	return nil
}
func publicIP(ip net.IP) bool {
	v4 := ip.To4()
	shared := v4 != nil && v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127
	return ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsUnspecified() && !shared
}

type HTTPFetcher struct{ client *http.Client }

func NewFetcher() *HTTPFetcher {
	transport := &http.Transport{Proxy: nil, ResponseHeaderTimeout: 20 * time.Second, TLSHandshakeTimeout: 10 * time.Second}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, stageError("dns_failure", "No document host address was found.")
		}
		for _, ip := range ips {
			if !publicIP(ip.IP) {
				return nil, stageError("unsafe_url", "The document host resolves to a private or reserved address.")
			}
		}
		dialer := net.Dialer{Timeout: 15 * time.Second}
		return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
	}
	return &HTTPFetcher{client: &http.Client{Transport: transport, Timeout: 45 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return stageError("too_many_redirects", "The document redirected too many times.")
		}
		return SafeURL(req.URL.String())
	}}}
}
func (f *HTTPFetcher) Fetch(ctx context.Context, raw string) (Document, error) {
	if err := SafeURL(raw); err != nil {
		return Document{}, err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", raw, nil)
	if err != nil {
		return Document{}, err
	}
	req.Header.Set("Accept", "application/pdf")
	req.Header.Set("User-Agent", "ClearCare-SBC/1.0")
	resp, err := f.client.Do(req)
	if err != nil {
		return Document{}, stageError("retrieval_failed", "The SBC could not be downloaded from its public source.")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return Document{}, stageError("document_unavailable", "The document source requires access, no longer exists, or returned an error.")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxDocumentBytes+1))
	if err != nil {
		return Document{}, stageError("retrieval_failed", "The SBC download was interrupted.")
	}
	if len(data) > MaxDocumentBytes {
		return Document{}, stageError("document_too_large", "The SBC exceeds the 10 MB document limit.")
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		return Document{}, stageError("not_pdf", "The source did not return a PDF SBC; it may be a login or landing page.")
	}
	return NewDocument(data, "application/pdf", resp.Request.URL.String()), nil
}

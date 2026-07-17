package doh

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
)

// fakeDoer returns a canned response and records the request it received.
type fakeDoer struct {
	status  int
	body    string
	lastURL string
	lastHdr http.Header
}

func (f *fakeDoer) Do(req *http.Request) (*http.Response, error) {
	f.lastURL = req.URL.String()
	f.lastHdr = req.Header
	return &http.Response{
		StatusCode: f.status,
		Body:       io.NopCloser(bytes.NewBufferString(f.body)),
		Header:     make(http.Header),
	}, nil
}

func TestQueryParsesAnswers(t *testing.T) {
	f := &fakeDoer{status: 200, body: `{
		"Status":0,"AD":true,
		"Answer":[
			{"name":"example.com.","type":1,"TTL":3600,"data":"93.184.216.34"},
			{"name":"example.com.","type":1,"TTL":3600,"data":"93.184.216.35"}
		],
		"Comment":"ok"
	}`}
	c := &Client{HTTP: f, UserAgent: "doh-lookup/test", SuppressECS: true}
	p, _ := ProviderByName("cloudflare", "")

	resp, err := c.Query(p, "example.com", "A", false)
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}
	if resp.Provider != "cloudflare" || resp.Endpoint == "" {
		t.Errorf("provenance not set: %+v", resp)
	}
	if resp.StatusText != "NOERROR" {
		t.Errorf("StatusText = %q, want NOERROR", resp.StatusText)
	}
	if !resp.Authenticated {
		t.Error("Authenticated = false, want true (AD flag)")
	}
	if len(resp.Answers) != 2 {
		t.Fatalf("got %d answers, want 2", len(resp.Answers))
	}
	if resp.Answers[0].Type != "A" || resp.Answers[0].Data != "93.184.216.34" {
		t.Errorf("answer[0] = %+v", resp.Answers[0])
	}
	// ECS suppression + Accept header wired correctly for Cloudflare.
	if !strings.Contains(f.lastURL, "edns_client_subnet=0.0.0.0%2F0") {
		t.Errorf("ECS param missing from URL: %s", f.lastURL)
	}
	if f.lastHdr.Get("Accept") != "application/dns-json" {
		t.Errorf("Accept = %q, want application/dns-json", f.lastHdr.Get("Accept"))
	}
}

func TestQueryNXDOMAIN(t *testing.T) {
	f := &fakeDoer{status: 200, body: `{"Status":3,"AD":false}`}
	c := &Client{HTTP: f}
	p, _ := ProviderByName("google", "")
	resp, err := c.Query(p, "nonexistent.invalid", "A", false)
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}
	if resp.StatusText != "NXDOMAIN" {
		t.Errorf("StatusText = %q, want NXDOMAIN", resp.StatusText)
	}
	if len(resp.Answers) != 0 {
		t.Errorf("got %d answers, want 0", len(resp.Answers))
	}
}

func TestQueryHTTPError(t *testing.T) {
	f := &fakeDoer{status: 429, body: "rate limited"}
	c := &Client{HTTP: f}
	p, _ := ProviderByName("cloudflare", "")
	if _, err := c.Query(p, "example.com", "A", false); err == nil {
		t.Fatal("expected error on HTTP 429, got nil")
	}
}

func TestNormalizeType(t *testing.T) {
	name, code, err := NormalizeType("aaaa")
	if err != nil || name != "AAAA" || code != 28 {
		t.Errorf("NormalizeType(aaaa) = %q,%d,%v", name, code, err)
	}
	if _, _, err := NormalizeType("BOGUS"); err == nil {
		t.Error("expected error for unknown type")
	}
}

func TestProviderByNameUnknown(t *testing.T) {
	if _, err := ProviderByName("quad9", ""); err == nil {
		t.Error("expected error for unknown provider")
	}
}

package doh

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Doer is the minimal HTTP surface the client needs; *http.Client satisfies
// it, and tests inject a fake.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Provider describes a DoH endpoint. The two built-ins are Cloudflare and
// Google; Endpoint may be overridden from config while keeping the provider's
// wire quirks (Accept header, boolean spelling).
type Provider struct {
	Name       string // "cloudflare" | "google"
	Endpoint   string // base URL of the JSON DoH API
	jsonAccept bool   // send Accept: application/dns-json (Cloudflare)
	boolTrue   string // spelling of a true query flag ("1" or "true")
}

// providers is the built-in registry. Cloudflare is the default (clear
// privacy stance; does not forward ECS). Google is the alternative.
var providers = map[string]Provider{
	"cloudflare": {Name: "cloudflare", Endpoint: "https://cloudflare-dns.com/dns-query", jsonAccept: true, boolTrue: "true"},
	"google":     {Name: "google", Endpoint: "https://dns.google/resolve", jsonAccept: false, boolTrue: "1"},
}

// ProviderByName returns a built-in provider, applying endpointOverride when
// it is non-empty. An unknown name is an error.
func ProviderByName(name, endpointOverride string) (Provider, error) {
	p, ok := providers[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return Provider{}, fmt.Errorf("unknown DoH provider %q (supported: cloudflare, google)", name)
	}
	if endpointOverride != "" {
		p.Endpoint = endpointOverride
	}
	return p, nil
}

// Answer is one normalized resource record from a DoH response.
type Answer struct {
	Name string `json:"name"`
	Type string `json:"type"` // mapped name, e.g. "A", "MX"
	TTL  int    `json:"ttl"`
	Data string `json:"data"`
}

// Response is the normalized result of one record-type query, carrying the
// provenance (provider + endpoint) that makes the lookup distinguishable.
type Response struct {
	Provider      string          `json:"provider"`
	Endpoint      string          `json:"endpoint"`
	Name          string          `json:"name"`   // queried name
	Type          string          `json:"type"`   // queried record type
	Status        int             `json:"status"` // DNS RCODE
	StatusText    string          `json:"status_text"`
	Authenticated bool            `json:"authenticated"` // DNSSEC AD flag
	Answers       []Answer        `json:"answers"`
	Comment       string          `json:"comment,omitempty"`
	Raw           json.RawMessage `json:"-"` // the resolver's raw JSON body (for --raw)
}

// wireResponse mirrors the Google/Cloudflare JSON DoH shape. It is decoded
// leniently: the two APIs differ in optional fields and neither is an RFC, so
// we normalize into Response in one place.
type wireResponse struct {
	Status int  `json:"Status"`
	TC     bool `json:"TC"`
	RD     bool `json:"RD"`
	RA     bool `json:"RA"`
	AD     bool `json:"AD"`
	CD     bool `json:"CD"`
	Answer []struct {
		Name string `json:"name"`
		Type int    `json:"type"`
		TTL  int    `json:"TTL"`
		Data string `json:"data"`
	} `json:"Answer"`
	Comment json.RawMessage `json:"Comment"` // string or array across providers
}

// Client issues DoH queries. SuppressECS asks the resolver not to forward an
// EDNS Client Subnet so the investigator's network is not leaked (honored by
// Google; Cloudflare does not forward ECS regardless).
type Client struct {
	HTTP        Doer
	UserAgent   string
	SuppressECS bool
}

// Query performs one record-type lookup. rrType is a canonical type name
// (already validated by NormalizeType). When cd is true the resolver returns
// records even if DNSSEC validation fails (checking disabled).
func (c *Client) Query(p Provider, name, rrType string, cd bool) (*Response, error) {
	u, err := url.Parse(p.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint %q: %w", p.Endpoint, err)
	}
	q := url.Values{}
	q.Set("name", name)
	q.Set("type", rrType)
	q.Set("do", p.boolTrue) // request DNSSEC data so the AD flag is meaningful
	if cd {
		q.Set("cd", p.boolTrue)
	}
	if c.SuppressECS {
		q.Set("edns_client_subnet", "0.0.0.0/0")
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if p.jsonAccept {
		req.Header.Set("Accept", "application/dns-json")
	} else {
		req.Header.Set("Accept", "application/json")
	}
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("doh request to %s: %w", p.Name, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read %s response: %w", p.Name, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned HTTP %d: %s", p.Name, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var wire wireResponse
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("decode %s response: %w", p.Name, err)
	}

	out := &Response{
		Provider:      p.Name,
		Endpoint:      p.Endpoint,
		Name:          strings.TrimSuffix(name, "."),
		Type:          rrType,
		Status:        wire.Status,
		StatusText:    RcodeText(wire.Status),
		Authenticated: wire.AD,
		Comment:       decodeComment(wire.Comment),
		Raw:           json.RawMessage(body),
	}
	for _, a := range wire.Answer {
		out.Answers = append(out.Answers, Answer{
			Name: strings.TrimSuffix(a.Name, "."),
			Type: TypeName(a.Type),
			TTL:  a.TTL,
			Data: a.Data,
		})
	}
	return out, nil
}

// decodeComment tolerates the Comment field being a string (Google) or an
// array of strings (some Cloudflare responses), returning a flattened string.
func decodeComment(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var arr []string
	if json.Unmarshal(raw, &arr) == nil {
		return strings.Join(arr, " ")
	}
	return ""
}

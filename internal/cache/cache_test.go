package cache

import (
	"encoding/json"
	"testing"
	"time"
)

func TestPutGetExpiry(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	now := time.Unix(1_700_000_000, 0)
	key := Key("cloudflare", "domain", "example.com", "a")
	payload := json.RawMessage(`{"ok":true}`)

	if err := s.Put(key, payload, now, 60*time.Second); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Fresh within TTL.
	if got, ok := s.Get(key, now.Add(30*time.Second)); !ok || string(got) != string(payload) {
		t.Errorf("Get within TTL: ok=%v got=%s", ok, got)
	}
	// Expired past TTL.
	if _, ok := s.Get(key, now.Add(90*time.Second)); ok {
		t.Error("Get past TTL returned a hit, want miss")
	}
	// Missing key.
	if _, ok := s.Get(Key("nope"), now); ok {
		t.Error("Get of missing key returned a hit")
	}
}

func TestCountAndClear(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	now := time.Unix(1_700_000_000, 0)
	for _, n := range []string{"a.example", "b.example", "c.example"} {
		if err := s.Put(Key("google", "domain", n, "a"), json.RawMessage(`1`), now, time.Minute); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}
	if s.Count() != 3 {
		t.Errorf("Count = %d, want 3", s.Count())
	}
	n, err := s.Clear()
	if err != nil || n != 3 {
		t.Fatalf("Clear = %d,%v want 3,nil", n, err)
	}
	if s.Count() != 0 {
		t.Errorf("Count after Clear = %d, want 0", s.Count())
	}
}

func TestKeySanitizes(t *testing.T) {
	// Colons (IPv6-ish) and slashes must not leak into the filename.
	k := Key("google", "ip", "2001:db8::1/64")
	for _, c := range k {
		if c == ':' || c == '/' {
			t.Fatalf("Key leaked unsafe char: %q", k)
		}
	}
}

package cache

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// record is the on-disk envelope. Each entry carries its own expiry so DNS
// TTLs are honored per-record.
type record struct {
	ExpiresAtUnix int64           `json:"expires_at_unix"`
	Result        json.RawMessage `json:"result"`
}

// Store is a per-lookup answer cache rooted at a directory.
type Store struct {
	Dir string
}

// Key builds a safe cache filename from canonical parts (e.g. provider, kind,
// name, types). Validated inputs contain no path separators; any character
// outside [a-z0-9._-] is replaced defensively so the key is always a valid
// single filename.
func Key(parts ...string) string {
	joined := strings.ToLower(strings.Join(parts, "_"))
	var b strings.Builder
	for i := 0; i < len(joined); i++ {
		c := joined[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '.', c == '-', c == '_':
			b.WriteByte(c)
		default:
			b.WriteByte('_')
		}
	}
	return b.String() + ".json"
}

// Get returns the cached raw result for key when it has not yet expired.
func (s *Store) Get(key string, now time.Time) (json.RawMessage, bool) {
	b, err := os.ReadFile(filepath.Join(s.Dir, key))
	if err != nil {
		return nil, false
	}
	var rec record
	if err := json.Unmarshal(b, &rec); err != nil {
		return nil, false // corrupt entries read as misses; Put overwrites them
	}
	if now.Unix() >= rec.ExpiresAtUnix {
		return nil, false
	}
	return rec.Result, true
}

// Put stores a raw result under key, expiring after ttl from now. The write is
// atomic (temp file + rename) so a crash never leaves a truncated entry.
func (s *Store) Put(key string, result json.RawMessage, now time.Time, ttl time.Duration) error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	rec := record{ExpiresAtUnix: now.Add(ttl).Unix(), Result: result}
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(s.Dir, key), b)
}

// Count returns the number of cached entries.
func (s *Store) Count() int {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			n++
		}
	}
	return n
}

// Clear removes every cached entry, returning the number removed.
func (s *Store) Clear() (int, error) {
	entries, err := os.ReadDir(s.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		if err := os.Remove(filepath.Join(s.Dir, e.Name())); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func writeAtomic(path string, b []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

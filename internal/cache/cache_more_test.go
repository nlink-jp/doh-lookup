package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCorruptEntryReadsAsMiss(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Dir: dir}
	key := Key("cloudflare", "domain", "corrupt.example", "a")
	if err := os.WriteFile(filepath.Join(dir, key), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get(key, time.Unix(1_700_000_000, 0)); ok {
		t.Error("corrupt entry should read as a miss")
	}
}

func TestClearNonexistentDir(t *testing.T) {
	s := &Store{Dir: filepath.Join(t.TempDir(), "does-not-exist")}
	n, err := s.Clear()
	if err != nil || n != 0 {
		t.Errorf("Clear on missing dir = %d,%v want 0,nil", n, err)
	}
	if s.Count() != 0 {
		t.Errorf("Count on missing dir = %d, want 0", s.Count())
	}
}

func TestPutCreatesDir(t *testing.T) {
	nested := filepath.Join(t.TempDir(), "a", "b", "c")
	s := &Store{Dir: nested}
	now := time.Unix(1_700_000_000, 0)
	if err := s.Put(Key("x"), []byte(`1`), now, time.Minute); err != nil {
		t.Fatalf("Put should create the dir: %v", err)
	}
	if s.Count() != 1 {
		t.Errorf("Count = %d, want 1", s.Count())
	}
}

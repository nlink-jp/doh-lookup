// Package cache is a per-lookup answer cache rooted at a directory. Unlike a
// fixed-TTL cache, each entry stores its own expiry so DNS TTLs are respected
// (floored by the configured minimum): a short-TTL record naturally expires
// sooner than a long-TTL one. Freshness lives in the record, not the file
// mtime, so it survives copies. The clock is supplied by the caller (the
// engine is the only clock reader), keeping the store deterministic and
// testable. Writes are atomic (temp file + rename).
package cache

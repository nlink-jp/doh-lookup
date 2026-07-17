// Package e2e holds live end-to-end tests that query the real Cloudflare and
// Google DoH endpoints. They are guarded by the `e2e` build tag so `go test
// ./...` (offline) never runs them; run them deliberately with:
//
//	make e2e          # or: go test -tags e2e -count=1 ./e2e/...
//
// This file has no build tag so the package always compiles (and reports "no
// test files" without the tag).
package e2e

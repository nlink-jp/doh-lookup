// Package config resolves runtime settings from an optional sectioned-TOML
// file and DOH_LOOKUP_* environment variables. Precedence is flag > env >
// file > built-in default (the flag layer is applied by the app package).
// There are no credentials: the DoH endpoints are public. The TOML reader is
// a deliberately tiny subset (headers + key = value); the record-type profile
// is a comma-separated string rather than a TOML array to keep the reader
// dependency-free.
package config

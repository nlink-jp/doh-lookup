// Package mcp is a zero-dependency stdio MCP server (JSON-RPC 2.0) exposing
// doh-lookup's engine as the tools lookup, cache_status, and get_usage. It
// shares the engine with the CLI so their behaviour cannot diverge.
// Diagnostics must go to stderr only; stdout carries the protocol.
package mcp

// Package mcp is doh-lookup's stdio MCP server, speaking both eras of the
// protocol through nlink-jp/mcp's server.ToolServer (ADR-0001), exposing
// doh-lookup's engine as the tools lookup, cache_status, and get_usage. It
// shares the engine with the CLI so their behaviour cannot diverge.
// Diagnostics must go to stderr only; stdout carries the protocol.
package mcp

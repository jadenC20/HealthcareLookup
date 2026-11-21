# ClearCare MCP connection (initial scaffold)

Status: implemented as an opt-in Go handler, **not mounted or deployed by default**. No tests or verification suites were run for this change.

The handler implements stateless JSON responses for the MCP 2025-11-25 Streamable HTTP protocol: `initialize`, `ping`, `tools/list`, `tools/call`, initialized/cancelled notifications. GET returns 405; there are no sessions or SSE streams. No new dependencies.

## Initial tools

- `clearcare_connection_info`: reports connection capabilities, no personal information.
- `clearcare_insurance_context`: optional sensitive read of the signed-in user's existing insurance context, including its coverage cautions. Disabled unless explicitly configured. No raw cards, member IDs, file downloads, account enumeration, write tools, AI processing, or bill records are exposed.

The endpoint requires a Supabase user bearer token on **every request**, verified through the existing account service. Cookies and deployment proxy secrets alone cannot authorize MCP. Existing per-user ownership checks remain in place; callers cannot provide a different user ID. Origin validation uses an exact allowlist. Never log access tokens, tool results, or medical information.

## Integration

In the Go API entry point, import `clearcare/mcpserver` and mount only behind an explicit feature flag:

```go
if os.Getenv("CLEARCARE_MCP_ENABLED") == "1" {
    mux.Handle("/mcp", mcpserver.New(mcpserver.Config{
        Authenticate: accounts.Me,
        // Leave sensitive reads disabled for initial connector setup.
        // InsuranceContext: accounts.InsuranceContext,
        // AllowSensitiveReads: true,
        AllowedOrigins: []string{"https://clearcare.pages.dev"},
    }))
}
```

Keep the existing backend origin and deployment proxy-secret middleware. The frontend proxy must explicitly permit `/api/mcp` and forward `Authorization`, `MCP-Protocol-Version`, `Accept`, and `Content-Type`; its fixed backend routing must not accept arbitrary upstream targets. Only the proxy supplies its deployment secret. Clients must connect over HTTPS.

Controlled first-party clients may use a short-lived authenticated user's token in the `Authorization: Bearer ...` header. Do not paste real tokens in chat, committed connector configuration, or public examples. This is not an API-key login flow and must not accept Supabase service-role or publishable keys as user credentials.

## Required before general-purpose assistant connections

This is a scaffold, **not a completed remote OAuth connector**. Most hosted AI assistants need a browser authorization flow rather than a manually supplied user token. Add:

1. A dedicated MCP authorization/resource boundary, OAuth protected-resource metadata, issuer discovery, PKCE, audience validation and narrowly scoped delegated tokens. Do not simply forward third-party OAuth tokens to unrelated services or reuse a Supabase service key for authorization.
2. Per-user connection grants and explicit consent for each sensitive tool scope, expiration, revocation and a profile UI showing active connections. A model-generated `consent: true` argument is not user authorization.
3. Per-user/client rate limiting, metadata-only audit events, abuse protection and assistant-client interoperability/security validation before enabling publicly.
4. Visible disclosure that retrieved health information will be shared with the connected assistant. Preserve insurance uncertainty warnings, minimize fields, treat external text as untrusted, and keep write/delete/upload tools unavailable until separately designed and approved.

Until these exist, keep sensitive reads off and restrict any pilot to trusted first-party clients. The MCP code does not itself opt users into AI processing or make any model calls.

References: [MCP transports](https://modelcontextprotocol.io/specification/2025-11-25/basic/transports), [MCP tools](https://modelcontextprotocol.io/specification/2025-11-25/server/tools), [MCP authorization](https://modelcontextprotocol.io/specification/2025-11-25/basic/authorization).

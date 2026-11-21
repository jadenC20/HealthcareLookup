# ClearCare MCP server (Cloudflare Workers)

Endpoint: `https://clearcare-mcp.agrigine-mcp.workers.dev/mcp`. The code lives in [mcp/](../mcp/): `worker.mjs` is the protocol handler and `tools.mjs` is the tool table.

It speaks stateless MCP Streamable HTTP with JSON responses (protocol 2025-11-25, 2025-06-18 and 2025-03-26). Supported methods: `initialize`, `ping`, `tools/list`, `tools/call`, `resources/list`, `resources/read`, `prompts/list` and `prompts/get`. It has no sessions and no SSE.

## Auth

Private tools need the user's ClearCare (Supabase) access token in `Authorization: Bearer …`. The Worker sends that token to the Render backend along with the deployment proxy secret, so the backend still runs its own per-user checks. Callers can't pick a user ID, an upstream host or a route outside the tool table. The public tools work without a token: health, sample bill, data status, providers, service prices and quick bill check.

## Coverage

Each backend route has a tool (47 in all). The groups are account and profile, preferences, insurance card/SBC workflow, bill extraction/analysis/reviews/PDFs, care search and recommendations, drug lookup and comparison, medicine price search with nearby-pharmacy and delivery suggestions, and jobs and saved results.

- Long-running tools (extract bill, analyze bill, recommend care, compare prescription, find medicine prices) wait up to about 50 s for their job by default. Pass `wait: false` to get the job back right away, then use `clearcare_get_job`.
- For file uploads, send base64 in `*_file_base64` plus `*_content_type` (PDF/PNG/JPEG, 10 MB max).
- Downloads come back as embedded `resource` blobs.
- Deleting the account requires `confirm: "DELETE MY ACCOUNT"`.

Resources: `clearcare://profile`, `preferences`, `insurance`, `reviews`, `results`, `jobs` and `health`. Prompts: `cheapest_medicine_near_me`, `review_my_bill` and `find_care`.

## Deploy

```bash
cd mcp && npx wrangler deploy
npx wrangler secret put CLEARCARE_PROXY_SECRET   # same value as Render/Pages
```

`CLEARCARE_API_URL` and `MCP_ALLOWED_ORIGINS` are set in `mcp/wrangler.toml`.

## Not yet done

The server has no OAuth authorization server. Hosted connectors that require a browser OAuth flow need one to be added before they can connect. Until then, use clients that let you set a Bearer header.

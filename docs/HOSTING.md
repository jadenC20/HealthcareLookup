# Hosted deployment preparation

Target: Cloudflare Pages frontend and same-origin edge proxy → Render Go API → Supabase Postgres and private Storage. Supabase Auth remains the login authority.

## Status

Implemented: Postgres adapter, isolated `clearcare` schema, parameter translation, transaction-pooler-compatible connections; Render Blueprint; explicit public-asset Pages build; authenticated edge-to-backend proxy; configurable origins and Secure legacy cookies.

Private Supabase Storage integration is implemented with immutable object keys, authenticated downloads, and a private object journal. The database retains object references rather than document bytes in hosted mode. Older replaced files remain journaled until insurance/account deletion; automated orphan expiry is a follow-up. Existing local SQLite records are not automatically migrated. Do not use real patient records for initial deployment.

## Cloudflare Pages

Connect the repository with build command `npm ci && npm run build:pages` and output directory `dist`. This copies only the explicit public asset list, not backend code, local databases or environment files. Advanced-mode `_worker.js` implements the API proxy and public auth config endpoint.

Runtime variables:

- `CLEARCARE_API_URL`: exact HTTPS Render origin, without a path.
- `CLEARCARE_PROXY_SECRET`: a strong random secret, identical on Render. Store as a secret, not in source control.
- `SUPABASE_URL`: project HTTPS origin.
- `SUPABASE_PUBLISHABLE_KEY`: public publishable key only.

## Render

Create the service from the repository-root `render.yaml`. It builds Go from `backend/goapp`, binds to `0.0.0.0:$PORT`, and checks `/health`. The current dependency set requires Go 1.25 or newer.

Set the prompted environment variables through the hosting dashboard:

- `DATABASE_URL`: Supabase Postgres connection URI with TLS required (`sslmode=require`), using the transaction pooler where IPv4 connectivity requires it. Prefer a dedicated database role confined to the `clearcare` schema. Never expose this to Pages or browser JavaScript.
- `CLEARCARE_ALLOWED_ORIGINS`: exact production Pages origin. Add another exact origin only when intentionally enabling it; do not use a wildcard preview origin.
- `CLEARCARE_PROXY_SECRET`: shared with the Pages Function. Required for every backend endpoint except `/health`; this is additional protection, not a replacement for user authentication.
- `SUPABASE_URL`, `SUPABASE_PUBLISHABLE_KEY`: same public auth configuration.
- `CLEARCARE_AI_PROVIDER=cloudflare`, `CLOUDFLARE_AI_MODEL=@cf/meta/llama-4-scout-17b-16e-instruct`, and `CLOUDFLARE_AI_URL=https://clearcare-ai.agrigine-mcp.workers.dev/internal/ai`. The private AI Worker uses an `AI` binding and the same `CLEARCARE_PROXY_SECRET` as Render; no expiring API token is needed by the backend. See [AI provider configuration](AI_PROVIDER.md).
- `CLEARCARE_DOCUMENT_STORE=supabase`, `SUPABASE_STORAGE_BUCKET=clearcare-private`, and `SUPABASE_SERVICE_ROLE_KEY`: backend-only private storage configuration. The service refuses a public bucket.

Do not add the `clearcare` schema to Supabase's exposed Data API schemas. Do not grant its private business tables to `anon` or `authenticated` database roles. Access is through the Go API's verified per-user authorization.

## Supabase Auth

Add the production Pages URL to the Auth redirect allowlist and set the production Site URL as appropriate. Keep any intentionally used localhost/Tailscale redirects. A hosted origin has its own browser login session.

## Still needed before release

1. Add scheduled cleanup for obsolete immutable uploads; explicit insurance/account deletion already removes journaled files.
2. Configure the hosting accounts and backend secrets securely, then deploy.
3. Observe successful builds, startup migrations and live health/auth behavior before calling the app live. No local tests have been run for this preparation.
4. Separately review handling of sensitive health information and provider arrangements before accepting real patient data.

References: [Cloudflare advanced-mode Pages Functions](https://developers.cloudflare.com/pages/functions/advanced-mode/), [Render Blueprint reference](https://render.com/docs/blueprint-spec), [Supabase private buckets](https://supabase.com/docs/guides/storage/buckets/fundamentals).

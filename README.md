# ClearCare

ClearCare uses a JavaScript/HTML/CSS frontend, a local Node HTTP proxy, a Go healthcare API with SQLite, Supabase authentication, and OpenRouter for document reading and explanations.

## Run this checkout

Requires Node.js 20.12+ and Go 1.24+.

The frontend server loads ignored root `.env` configuration. Use `.env.example` for `SUPABASE_URL` and the public `SUPABASE_PUBLISHABLE_KEY` (or legacy `SUPABASE_ANON_KEY`). Secret/service_role keys are rejected by the frontend configuration endpoint. The OpenRouter key is never sent to the browser.

In the macOS/zsh terminal that will run Go:

```zsh
read -rs 'OPENROUTER_API_KEY?Paste the complete OpenRouter API key: '
export OPENROUTER_API_KEY
export CLEARCARE_AI_PROVIDER=openrouter
npm run backend
```

Paste the full original OpenRouter key when prompted, then press Enter. The input is hidden and kept out of shell command history; do not replace characters with asterisks or paste a placeholder. The launcher loads Supabase settings from root `.env` and inherits `OPENROUTER_API_KEY` from the terminal. It runs the updated `backend/goapp` in this checkout. Stop older servers on port 8000 first. Changes to Go source or keys require restarting it.

Production AI uses Cloudflare Workers AI through a private Worker, configured with `CLEARCARE_AI_PROVIDER=cloudflare`, `CLOUDFLARE_AI_MODEL`, `CLOUDFLARE_AI_URL`, and the shared backend/Worker secret. The OpenRouter terminal setup above is an optional compatibility mode. See [AI provider configuration](docs/AI_PROVIDER.md) and [hosted deployment](docs/HOSTING.md).

In another terminal:

```sh
npm run dev
```

Open http://127.0.0.1:4182 (or the printed port). `PORT` configures the frontend; `CLEARCARE_API_URL` defaults to http://127.0.0.1:8000. Go binds loopback only; `CLEARCARE_ADDR` can select another loopback port.

In Supabase Authentication → URL Configuration, allow the actual frontend origin and email redirect URL, such as `http://127.0.0.1:4182/**`. PKCE confirmation must finish in the browser where signup began. Email confirmation settings remain controlled by Supabase.

## One account for profile and insurance

Sign up/sign in uses the official pinned Supabase SDK. Passwords go directly to Supabase. The browser obtains the current access token for protected healthcare requests; Node forwards it to Go. Go verifies it against the configured project's `/auth/v1/user` endpoint and requires a confirmed, non-anonymous email identity. It opens a healthcare profile using a namespaced Supabase subject ID, with no second password or automatic email-based merging of old accounts. Invalid bearer credentials never fall back to a legacy cookie.

The account page provides display name, city, **Insurance plan**, insurance card/SBC tools, saved reviews and PDF export. The plan label is not verified coverage. Healthcare data is stored in the local SQLite database. The SDK manages authentication tokens in browser storage; healthcare documents are not stored there by the frontend. Deleting healthcare data does not delete the Supabase login account. Old cookie accounts remain supported by Go for compatibility, but the main UI uses Supabase only.

## AI bill review

1. Sign in and connect the healthcare profile.
2. On Understand my bill, select a PDF/PNG/JPEG medical bill (max 10 MB), consent, and choose **Read bill with AI**.
3. Go sends it through OpenRouter and returns itemized fields, supporting quotes/pages, missing fields and warnings. Unknown amounts stay null and must be confirmed. The original bill file is not saved to the healthcare database by this extraction endpoint.
4. Confirm/edit the fields or enter them manually. Optionally enter EOB data as JSON.
5. Consent and choose **Review with AI**. Go combines bill checks with the account's saved insurance context; OpenRouter returns explanations tied to bill line IDs, questions, limitations and an inquiry draft. Basic checks are separately available without AI.
6. Save/export the review. Successful AI output is retained server-side by user and request hash for 24 hours so saving/exporting can include the same analysis. Saved reviews persist independently. AI findings are questions, not proof of errors or amounts owed.

Requests: `POST /documents/extract`, `POST /bills/ai-review`; basic checks use `POST /bills/analyze`.

## Insurance-aware care search

Describe the service or care you need in plain language, enter a city (defaults to the saved city when omitted), and choose a price type. Procedure codes are not part of the user workflow. With plan explanations enabled and consent, `GET /care/recommendations` loads sourced backend records and the signed-in user's saved plan and documented benefits. Cloudflare Workers AI explains relevant restrictions and questions, citing only returned listing and benefit indices. It does not generate prices or determine network/individual coverage. Empty datasets return a no-data state without inventing providers. Demo rows are excluded unless explicitly enabled. Standalone provider and price browsing remain available.

The directory still needs sourced `providers.csv` and `service_prices.csv` records. `CLEARCARE_DEMO=1` selects the backend's explicit demo dataset. An API key does not supply real provider pricing.

## Insurance document interpretation

Account uploads accept PDF/PNG/JPEG cards and SBC PDFs. The backend identifies plan attributes, discovers candidate SBC documents from public research or a configured local catalog, requires selection when ambiguous, and interprets benefits with supporting evidence. Upload an SBC directly when discovery has no supported match. Review cautions, missing fields and plan identity before relying on the output. Downloads, retry, correction and deletion are available.

## Connection and security

Browser requests use same-origin `/api`; Node's allowlisted proxy validates the browser origin and forwards JSON/multipart/PDF data and authorization headers to Go. Go contacts Supabase for identity verification and OpenRouter for AI operations. Supabase subjects scope profile, insurance, reviews and AI cache rows. Legacy cookie mutations still require CSRF. AI output references are validated, external-document fetching retains its existing SSRF safeguards, and provider errors never expose raw provider details or keys. AI calls use `store:false`; this does not represent a zero-retention guarantee. OpenRouter also forwards inputs to the selected inference provider; PDFs use the explicitly selected free parser described in the [AI provider guide](docs/AI_PROVIDER.md).

Root `.env` is ignored. Public config is limited to the Supabase project URL and publishable/anon key. Do not expose a secret/service_role or OpenRouter key in frontend code. This server is for local development; hosted deployment requires HTTPS, appropriate origins and deployment configuration.

## Validation and limitations

```sh
npm run check
npm test
cd backend/goapp
go test ./...
go vet ./...
```

Tests use local fixtures and mocked Supabase/AI-provider responses. Optional live insurance tests require explicit `CLEARCARE_LIVE_TEST=1`; normal tests do not create cloud accounts, send email or make external AI calls. A live key returning 401 must be fixed before real AI outputs can be verified.

Prescription savings accepts a medicine name and automatically searches published pharmacy and price-comparison sources. Supported price amounts include source quotes and links; otherwise the search returns links for checking prices directly. Strength, quantity and ZIP are not mandatory steps. Coupon activation, booking, advocacy, live maps, computed savings and remaining deductible are not implemented. No sample output is shown as a user's actual result. `assets/vendor/supabase.js` is committed for local startup without runtime CDN access; update it with `npm run vendor:auth` after updating the pinned package.

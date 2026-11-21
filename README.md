# ClearCare

ClearCare uses a JavaScript/HTML/CSS frontend, a local Node HTTP proxy, a Go healthcare API with SQLite, Supabase authentication, and OpenAI for document reading and explanations.

## Run this checkout

Requires Node.js 20.12+ and Go 1.24+.

The frontend server loads ignored root `.env` configuration. Use `.env.example` for `SUPABASE_URL` and the public `SUPABASE_PUBLISHABLE_KEY` (or legacy `SUPABASE_ANON_KEY`). Secret/service_role keys are rejected by the frontend configuration endpoint. The OpenAI key is never sent to the browser.

In the PowerShell terminal that will run Go:

```powershell
$key = Read-Host "Paste the complete OpenAI API key" -AsSecureString
$env:OPENAI_API_KEY = ([System.Net.NetworkCredential]::new("", $key).Password).Trim()
npm run backend
```

Paste the full original key. SecureString masks the display; do not replace characters with asterisks or paste a placeholder. The launcher loads Supabase settings from root `.env` and inherits `OPENAI_API_KEY` from the terminal. It runs the updated `backend/goapp` in this checkout. Stop older servers on port 8000 first. Changes to Go source or keys require restarting it.

In another terminal:

```powershell
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
3. Go sends it to OpenAI and returns itemized fields, supporting quotes/pages, missing fields and warnings. Unknown amounts stay null and must be confirmed. The original bill file is not saved to the healthcare database by this extraction endpoint.
4. Confirm/edit the fields or enter them manually. Optionally enter EOB data as JSON.
5. Consent and choose **Review with AI**. Go combines bill checks with the account's saved insurance context; OpenAI returns explanations tied to bill line IDs, questions, limitations and an inquiry draft. Basic checks are separately available without AI.
6. Save/export the review. Successful AI output is retained server-side by user and request hash for 24 hours so saving/exporting can include the same analysis. Saved reviews persist independently. AI findings are questions, not proof of errors or amounts owed.

Requests: `POST /documents/extract`, `POST /bills/ai-review`; basic checks use `POST /bills/analyze`.

## Insurance-aware care search

Enter a procedure code, city (defaults to saved city when omitted), and price type. With plan explanations enabled and consent, `GET /care/recommendations` loads sourced backend records and the signed-in user's saved plan and documented benefits. OpenAI explains relevant restrictions and questions, citing only returned listing and benefit indices. It does not generate prices or determine network/individual coverage. Empty datasets return a no-data state without inventing providers. Demo rows are excluded unless explicitly enabled. Standalone provider and price browsing remain available.

The directory still needs sourced `providers.csv` and `service_prices.csv` records. `CLEARCARE_DEMO=1` selects the backend's explicit demo dataset. An API key does not supply real provider pricing.

## Insurance document interpretation

Account uploads accept PDF/PNG/JPEG cards and SBC PDFs. The backend identifies plan attributes, discovers candidate SBC documents, requires selection when ambiguous, and interprets benefits with quotes and page numbers. Review cautions, missing fields and plan identity before relying on the output. Downloads, retry, correction and deletion are available.

## Connection and security

Browser requests use same-origin `/api`; Node's allowlisted proxy validates the browser origin and forwards JSON/multipart/PDF data and authorization headers to Go. Go contacts Supabase for identity verification and OpenAI for AI operations. Supabase subjects scope profile, insurance, reviews and AI cache rows. Legacy cookie mutations still require CSRF. AI output references are validated, external-document fetching retains its existing SSRF safeguards, and OpenAI errors never expose raw provider details or keys. OpenAI calls use `store:false`; this does not represent a zero-retention guarantee.

Root `.env` is ignored. Public config is limited to the Supabase project URL and publishable/anon key. Do not expose a secret/service_role or OpenAI key in frontend code. This server is for local development; hosted deployment requires HTTPS, appropriate origins and deployment configuration.

## Validation and limitations

```powershell
npm run check
npm test
cd backend/goapp
go test ./...
go vet ./...
```

Tests use local fixtures and mocked Supabase/OpenAI responses. Optional live insurance tests require explicit `CLEARCARE_LIVE_TEST=1`; normal tests do not create cloud accounts, send email or make billable OpenAI calls. A live key returning 401 must be fixed before real AI outputs can be verified.

Original visual examples remain inside closed, labeled preview panels. Prescription prices/coupons, booking, advocacy, live maps, computed savings and remaining deductible are not implemented. No sample output is shown as a user's actual result. `assets/vendor/supabase.js` is committed for local startup without runtime CDN access; update it with `npm run vendor:auth` after updating the pinned package.

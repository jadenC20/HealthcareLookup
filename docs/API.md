# Go API contract

Base address: `http://127.0.0.1:8000`. The existing development frontend proxies `/api` requests to this server; the routes below omit that proxy prefix. Responses are JSON unless a PDF/file is requested. Error bodies normally contain `detail`.

## Authentication

Register/login with JSON `{ "email": "...", "password": "...", "name": "..." }` (name is needed for registration). Responses include the user and CSRF token and set an HttpOnly session cookie. `GET /auth/me` restores the authenticated user/token. Send the session cookie on protected routes and `X-CSRF-Token` on protected mutations. Do not use another user's ID in requests: insurance ownership always comes from the session.

## Insurance routes

| Method/path | Input | Behavior |
|---|---|---|
| GET /profile/insurance-card | — | Card presence, update timestamp, lookup status |
| GET /profile/insurance-card/file | — | Original private card file |
| POST /profile/insurance-card | Multipart `file` | Save/replace PNG, JPEG, or PDF card; immediately extract and discover SBCs |
| DELETE /profile/insurance-card | — | Remove card and derived insurance/SBC |
| GET /profile/insurance | — | Current revision and workflow result |
| PUT /profile/insurance/details | JSON `revision`, `details` | Confirm/correct all plan fields and run discovery |
| POST /profile/insurance/select | JSON `revision`, `candidate_id` | Download/interpret a candidate from the current result |
| POST /profile/insurance/retry | JSON `revision` | Reinterpret saved SBC, or rerun extraction/discovery |
| POST /profile/insurance/sbc | Multipart `file`, `revision` | Save and interpret a user-supplied PDF |
| GET /profile/insurance/sbc | — | Download the private saved SBC PDF |
| DELETE /profile/insurance | — | Remove derived result/SBC; retain card |

All routes require authentication. Mutations require CSRF. Each file is limited to 10 MB; MIME/magic validation applies. The server caps total request bodies at 11 MB. SBC uploads accept only PDF. Card replacement deliberately starts a new workflow; no revision field is required for that action.

For other actions, send the latest revision from `GET /profile/insurance`. Use an empty revision only when no workflow exists. Details are a full replacement, not a patch:

```json
{
  "revision": "current-revision",
  "details": {
    "insurer": "Example Insurance",
    "plan_name": "Example Gold",
    "plan_id": "12345PA1234567-01",
    "network": "",
    "state": "PA",
    "year": 2026,
    "employer": ""
  }
}
```

Unknown strings are empty and unknown year is `0`. Fields are capped at 300 bytes; state is an uppercase two-letter code. Do not enter member numbers into `plan_id`; it describes the public plan/variant. Do not submit member IDs or group numbers as search attributes.

## Workflow response

Insurance mutations and status return `{ "revision": "...", "result": { ... } }`. `result` includes:

| Field | Meaning |
|---|---|
| status | `not_started`, `processing`, `interrupted`, `partial`, `needs_confirmation`, or `ready_with_caution` |
| may_be_inaccurate | Whether any caution is present |
| caution | Overall caution text, or empty string |
| details / details_confirmed | Extracted or user-confirmed public plan attributes |
| candidates | Source-backed candidate IDs, details, URL, source, source_url, match |
| selected | Chosen candidate, when applicable |
| document | MIME type, source URL, SHA-256, retrieval time; never the bytes |
| document_details | Identity extracted from the SBC, when available |
| benefits | Named terms with value, supporting quote, and 1-based PDF page |
| steps | Named executed stages and their status |
| cautions | Stage, code, explanation, missing fields/capability, and suggested action |
| updated | UTC timestamp |

Example caution fragment when OpenAI is not configured:

```json
{
  "status": "partial",
  "may_be_inaccurate": true,
  "caution": "This may be inaccurate because parts of the automated process are missing or could not be verified.",
  "cautions": [{
    "step": "card_extraction",
    "code": "openai_not_configured",
    "message": "OpenAI is not configured.",
    "missing": ["card_extraction"],
    "action": "Retry this step, correct the plan details, or upload the SBC manually."
  }]
}
```

Use codes and missing fields for application behavior; exact message prose may change. No UI changes are included in this implementation. A future frontend must show the overall caution and each missing part rather than rendering an empty field as zero coverage.

Benefits are limited to deductible, out_of_pocket_limit, primary_care, specialist, emergency_room, hospital_stay, prescription_drugs, and exclusions. Terms lacking a value, quote, or valid page are withheld. Conflicting document identities withhold all benefits. Quotes/page references are model-extracted and require review; they are not independently checked by a second PDF parser.

`ready_with_caution` means some source-supported terms were extracted, not that enrollment or payment has been verified. Active coverage, remaining deductible, provider network membership, and claim approval remain explicitly unverified. An SBC is not an EOB and does not expose live member balances.

## Selection and failures

Only a unique exact catalog variant with confirmed plan details, state, and coverage year can auto-proceed. Web candidates and ambiguous/base-ID matches require explicit selection. The selection endpoint accepts a returned candidate ID, never an arbitrary download URL. Downloaded URLs must use public HTTPS; private addresses, unsafe redirects, HTML/login responses, and oversized files are rejected.

Partial processing results return HTTP 200 and are persisted. Inspect their cautions. Invalid input uses 400/415/422, unauthenticated access 401, CSRF failures 403, missing resources 404, oversized files 413, stale revisions/concurrent work 409, and storage failures 500. On 409 reload the current state before attempting another action.

## Other existing routes

| Area | Routes |
|---|---|
| Health/sample | GET /health, GET /sample |
| Accounts | POST /auth/register, POST /auth/login, GET /auth/me, POST /auth/logout, PATCH /profile, DELETE /account |
| Bill analysis | POST /bills/analyze |
| Bill extraction placeholder | POST /documents/extract returns 501 after authentication/CSRF |
| Saved reviews | POST /reviews, GET /reviews, GET /reviews/{id}, DELETE /reviews/{id} |
| PDF reports | POST /reports/pdf, GET /reviews/{id}/pdf, GET /account/pdf |
| Care data | GET /providers, GET /prices/compare, GET /data/status |
| Reload | POST /data/reload returns 403; restart to reload data |

Bill input contracts are defined in [billreview](../backend/goapp/billreview/) and illustrated by the [sample request](../backend/sample/bill_with_problems.json). Care query parameters and response shapes are implemented in [care.go](../backend/goapp/caresearch/care.go). No prescription-comparison routes or live insurer eligibility endpoints exist yet.

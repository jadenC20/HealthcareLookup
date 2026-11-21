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

All insurance routes require authentication. Mutations require CSRF. Each file is limited to 10 MB; MIME/magic validation applies. The server caps total request bodies at 21 MB to allow a bill and optional EOB together. SBC uploads accept only PDF. Card replacement deliberately starts a new workflow; no revision field is required for that action.

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
| Bill extraction alias | POST /documents/extract processes multipart uploads after authentication/CSRF; use /bill-review/extract for saved results |
| Saved reviews | POST /reviews, GET /reviews, GET /reviews/{id}, DELETE /reviews/{id} |
| PDF reports | POST /reports/pdf, GET /reviews/{id}/pdf, GET /account/pdf |
| Care data | GET /providers, GET /prices/compare, GET /data/status |
| Reload | POST /data/reload returns 403; restart to reload data |

Existing manual bill input contracts are defined in [billreview](../backend/goapp/billreview/) and illustrated by the [sample request](../backend/sample/bill_with_problems.json). Care directory query parameters are implemented in [care.go](../backend/goapp/caresearch/care.go). The new product routes are documented below. No live insurer eligibility endpoint exists.

## Implemented product routes

These routes require the session cookie. POST/PUT/DELETE also require `X-CSRF-Token`. Automated product operations are limited to six per minute per account and bounded by four minutes. Long-running product POST routes return HTTP 202 with `{ "job": {...}, "duplicate": false }`. If an identical request is already queued/running for that user and feature, the existing job returns with `duplicate: true`.

| Method/path | Purpose |
|---|---|
| POST /bill-review/extract | Multipart `bill` (or `file`) and optional `eob`; PDF, PNG or JPEG; 10 MB each |
| POST /bill-review/analyze | Confirmed review request; deterministic checks, benchmarks, coding flags, AI explanation |
| GET /bill-review/observations?limit=25&cursor=... | Paginated current-user charge observations |
| POST /care-search/recommend | Preferences or `{}` to use the saved profile; returns ranked candidates and public research |
| GET /prescription-comparison/drugs?name=... | RxNorm lookup; select an exact clinical/branded product |
| POST /prescription-comparison/compare | Exact RxCUI, quantity, ZIP and price type comparison |
| GET /jobs?limit=25&cursor=... | Paginated current-user jobs |
| GET /jobs/{id} | Job progress, error, saved result ID and completed result |
| DELETE /jobs/{id} | Cancel a queued/running job |
| GET /profile/preferences | Read optional patient preferences |
| PUT /profile/preferences | Replace patient preferences |
| GET /results?limit=25&cursor=...&feature=... | Paginated private result summaries |
| GET /results/{id} | `{id, feature, created, result}` |
| DELETE /results/{id} | Delete result and its linked observations |

Jobs move through `queued`, `running`, `completed`, `failed`, `canceled`, or `interrupted`. Progress is an integer from 0–100 and `stage` is currently `queued`, `processing`, `saving_result`, `completed`, `failed`, `canceled`, or `service_restarted`. `poll_url` and `cancel_url` are included. Completed jobs contain `result_id` and `result`. Job state is durable, but request bodies are intentionally not stored; a restart marks unfinished work interrupted. GET drug lookup is authenticated but is not saved as a feature result.

### Bill Review

Extraction returns `documents`, `requires_confirmation: true`, and `uncertainty`. Monetary fields/quantity may be null; preserve unknowns and confirm them rather than converting to zero. Raw uploads are not saved; extracted private results are. Each line retains a supporting page/quote. Submit confirmed data using the existing bill/EOB request structure:

```json
{
  "confirmed": true,
  "facility_id": "1234567890",
  "state": "PA",
  "setting": "office",
  "payer": "",
  "plan": "",
  "request": {
    "bill": {
      "provider_name": "Example practice",
      "lines": [{"line_id":"1","service_date":"2026-11-01","description":"Office visit","code":"99213","quantity":1,"charge":150,"modifiers":[]}]
    },
    "eob": null
  }
}
```

The facility identifier is an NPI, never a patient account or phone number; the example is illustrative, not verified registry data. Leave it empty if unknown; observations are only created when it is supplied and a benchmark comparison exists. Output contains deterministic `analysis`, separate `comparisons`, `research`, `explanation`, and `uncertainty`. Each comparison has signed `over_under_benchmark` and its reference. Multiple applicable reference types remain separate; no aggregate refund is computed.

### Care Search and patient preferences

```json
{
  "specialty": "Family Medicine",
  "city": "Philadelphia",
  "state": "PA",
  "language": "Spanish",
  "insurance_plan": "",
  "telehealth": true,
  "accessibility": ["wheelchair access"],
  "demographics": {"age_group":"65+","gender":"woman","race_ethnicity":""},
  "use_demographics": true
}
```

Demographics belong to the patient. Empty optional demographic values are allowed; `use_demographics` defaults false. All nonempty selected fields must match a self-reported cohort. No matching cohort produces `demographic_evidence_missing`; it does not exclude providers. A recommendation request overlays supplied top-level fields on saved preferences. Supplying a demographics object replaces that whole object. An empty insurance plan may be filled from confirmed imported plan details.

Results include NPI, practice address, specialty, match score, reasons, aggregate `review_evidence` and source-backed `preference_evidence`. Scores are deterministic evidence-match heuristics, not outcome probabilities. Only language/telehealth/accessibility evidence accepted from exact returned source URLs can augment preference scores. Current insurance network status and appointments remain unverified.

### Prescription Comparison

```json
{"rxcui":"selected-rxnorm-id","quantity":30,"zip":"19103","price_type":"cash","current_price":25}
```

Use the actual numeric RxCUI returned by lookup; the example is a placeholder. `price_type` must be `cash` or `coupon`; insurance quotes are unsupported. Quantity must be 1–1000. `current_price` is optional and must describe the same product, quantity and price type. Returned `potential_savings` is signed current price minus offer price, not guaranteed savings. No therapeutic substitutions are performed.

The completed job result contains the canonical drug, offers sorted by price, research sources and cautions. Each offer carries price type, currency, conditions, quote, source URL and retrieval timestamp. Empty offers are a valid partial result. Retry after correcting inputs or service availability by submitting again; successful runs are saved independently.

Charge observations additionally require the supplied facility NPI to resolve through the public registry. `facility_verified` reports that identifier check; it does not prove the facility issued the uploaded bill. Failed identity lookup leaves the review available with a caution and skips observation storage.

## Missing-data fallbacks

Imported reference files and demographic-review cohorts are optional enhancements, never prerequisites for a result. Bill Review tries public-source price extraction for unmatched lines, retaining a source quote and `reference.origin: "public_search"`. These rates carry an explicit verification caution and are excluded from charge-observation storage. If no applicable rate is available, `review_basis` identifies bill arithmetic, duplicate/description checks and available EOB reconciliation; an explanation and inquiry draft still return without a price estimate. Failed AI explanation also has a deterministic text fallback.

Care Search returns `matching_basis`; missing demographic evidence selects `general_matching_fallback` while specialty/location and available service preferences continue. If registry/local candidates are unavailable, source-supported public directory candidates may be extracted into provider items with `identity_status: "requires_confirmation"` and a caution. Demographic similarities are never invented. If RxNorm lookup fails, Prescription Comparison returns `fallback: "public_drug_research"` with source cards and cautions; numeric offers remain withheld until exact identity is established.

## Grid-item response contract

Search and review results expose an explicit `grid` object containing `layout: "grid"`, `schema_version: "1"`, `total`, and `items: []`. `research_grid` contains source cards separately. Saved-result envelopes keep these objects inside `result`. Existing domain arrays (`results`, `offers`, `comparisons`, `findings`) remain for compatibility. Each product builds its grid directly from typed domain records; the encoder does not infer item types from JSON keys.

Each item has:

- `id`: deterministic content identifier; insurance candidates retain their selectable candidate ID.
- `kind`: `provider`, `drug`, `prescription_offer`, `bill_comparison`, `bill_finding`, or `research_source`.
- `title` and `subtitle`: display labels.
- `fields`: ordered `{key, label, value, status}` entries; numbers remain numbers, missing values are null with status `unknown`.
- `sources`: `{label, url}` links.
- `cautions`: warnings relevant to that item, such as public provider identity or price confirmation.
- `data`: original typed domain record, including IDs required for follow-up actions.

Public web research returns `research.sources` and a separate `research_grid` of source-link cards. These remain separate from verified provider/drug/price items and carry `source_requires_review`; a research link must not be rendered as a verified offer. Raw search prose stays internal for extraction. Supporting explanations remain separate from the primary grid. Empty collections return `items: []` and `total: 0`; collection-wide cautions remain in `uncertainty`.

No frontend changes are included. Clients should render text as text, not injected HTML. Product IDs use domain identifiers where available and content/source identity for fallback records.

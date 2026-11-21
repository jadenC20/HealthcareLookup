# ClearCare

ClearCare helps users review medical bills, find suitable doctors, and find less expensive prescribed medicine. AI is integral to the intended experience: it reads documents and explains evidence. Pricing and coverage conclusions must come from identifiable sources, not model memory.

The active backend is Go. This iteration implements the **insurance card → plan discovery → Summary of Benefits and Coverage (SBC) → cited benefit interpretation** backend workflow. No frontend work was included in this iteration. The existing profile can upload a card, but its old text and controls do not yet expose the automated workflow, candidate selection, or cautions. Use the API described below to exercise the complete backend.

## Product and implementation status

| Area | Intended experience | Current implementation |
|---|---|---|
| Bill Review | Upload bill/invoice/receipt image or PDF, optionally add an EOB beside it; AI extracts charges and compares them with grounded references | Manual input, deterministic checks, saved reviews, and PDF export. Bill extraction and reference-based over/undercharge estimation are not implemented |
| Care Search | Find the best-fit doctor using the user's profile and preferences | CSV-backed provider directory and procedure-price comparison; personalized AI ranking is not implemented |
| Prescription Comparison | Find cheaper prescribed medicine | Not implemented; the existing UI displays an availability notice |
| Insurance profile | Upload a card, automatically find an SBC, interpret benefits, expose uncertainty | Implemented backend with OpenAI, optional CMS catalogue, web search fallback, candidate selection, manual SBC upload, persistence, citations, and structured cautions |
| Accounts | Private saved information and local sessions | Registration, login/logout, profile, saved reviews, deletion, PDF reports |

See [the product specification](docs/PRODUCT.md) for confirmed decisions and unresolved product questions. See [architecture](ARCHITECTURE.md) for frontend and backend graphs.

## Run locally

Requires Go 1.24+ and Node.js. No Python environment or OCR installation is needed by the active backend.

```sh
# Supply your OpenAI key in the shell environment (do not commit it).
export OPENAI_API_KEY='your-key'
# Optional: override the model available to your API account.
export OPENAI_MODEL='gpt-4.1'
export OPENAI_SEARCH_MODEL='gpt-4.1'
npm run dev -- --demo
```

The launcher builds a Go executable, starts it on `127.0.0.1:8000`, and starts the static frontend/API proxy on `127.0.0.1:4173`. Open http://127.0.0.1:4173. `--demo` selects fictional care-search CSVs; it does **not** mock or disable OpenAI. Insurance uploads can make paid API calls when a key is configured.

If Go is not on PATH, set `CLEARCARE_GO` to its executable. Dependencies are pinned in [go.mod](backend/goapp/go.mod) and [go.sum](backend/goapp/go.sum); the first build downloads them. The application reads process environment variables; it does not automatically load `.env` files. See [.env.example](.env.example) and [operations](docs/OPERATIONS.md).

To start only the backend:

```sh
cd backend/goapp
CLEARCARE_DEMO=1 go run .
```

## Insurance automation

1. An authenticated card upload starts extraction and plan discovery.
2. The backend extracts public plan attributes from the card. It does not include member IDs or card images in web-search requests.
3. It tries the configured CMS catalogue, then OpenAI web search when the catalogue cannot establish a unique exact variant.
4. The user can correct/confirm details and select a source-backed candidate when the match is ambiguous.
5. The backend downloads the selected public PDF, preserves its hash/source, and extracts cited benefits. A user-uploaded SBC is the fallback for unavailable documents.
6. Every incomplete or unverifiable result carries `may_be_inaccurate`, a caution message, and step-specific missing information and recovery actions.

A missing API key, unavailable model, failed search, blocked document, or unreadable field results in a stored partial result—not fabricated coverage. The API always distinguishes SBC terms from current member eligibility or deductible balances. [Complete endpoint contract and examples](docs/API.md).

## Storage and external processing

The default SQLite file is `backend/clearcare.db`. It stores accounts, hashed session tokens, saved reviews, insurance cards, lookup results, and retrieved/uploaded SBCs. `CLEARCARE_DB` overrides the path. The existing Python account tables and scrypt hashes remain readable; the new insurance workflow table is created additively. Back up existing data before switching versions.

Card and SBC interpretation sends document contents to OpenAI with Responses API `store: false`. This is not a promise of zero provider retention. Web search receives the public plan fields, and document retrieval contacts public source hosts. No raw provider error bodies, keys, or documents are written to application logs. Deleting a card removes its lookup and saved SBC; account deletion removes all owned card/workflow records. [Storage and deletion details](docs/OPERATIONS.md).

This remains a loopback prototype, with no production compliance certification, account recovery, or live eligibility connection. Compliance work is a later project phase.

## Validation

```sh
cd backend/goapp
go test ./...
go vet ./...
```

The default tests use synthetic inputs and simulated external services. Opt-in paid provider verification:

```sh
CLEARCARE_LIVE_TEST=1 go test ./insurance -run TestLiveOpenAI -v
```

That test sends synthetic PDFs and a public plan query, never real patient data. `npm run check` validates the existing JavaScript syntax. The previous Python source and tests remain as historical reference and are not the active backend.

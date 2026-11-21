# Go API

Run from this directory:

```sh
go run .
go test ./...
go test -race ./...
go vet ./...
```

The server uses Go's HTTP server and SQLite. Defaults are `127.0.0.1:8000`, database `../clearcare.db`, and care-search data `../data`. It adds missing tables at startup and preserves existing account/password and review formats.

## Packages

| Package | Responsibility |
|---|---|
| [account](account/) | Identity, profile, card/SBC storage, revision-guarded insurance actions, saved reviews, PDF export |
| [insurance](insurance/) | OpenAI Responses requests, structured extraction, catalog/web discovery, public PDF retrieval, interpretation, cautions |
| [billreview](billreview/) | Deterministic analysis of manually supplied bill and EOB data |
| [caresearch](caresearch/) | CSV-backed provider and procedure-price search |

[main.go](main.go) wires routes and configuration. No prescription service is implemented. The old Python backend is not required to run this server.

## Insurance automation

Set `OPENAI_API_KEY` before startup. `OPENAI_MODEL` defaults to `gpt-4.1`; `OPENAI_SEARCH_MODEL` defaults to the same model. Both must support the capabilities used by their requests. Optional `CLEARCARE_PLAN_CATALOG` points to a CMS Plan Attributes CSV or the documented normalized catalog.

Card uploads automatically extract public plan attributes and search for SBC candidates. Confirmed exact catalog variants can proceed automatically; ambiguous or web-discovered candidates require selection through the API. Selected PDFs are downloaded and interpreted with source quotes and page numbers. Manual SBC upload and retry are supported. Missing configuration, unavailable sources, incomplete terms, and mismatches are represented explicitly in the result.

```sh
CLEARCARE_LIVE_TEST=1 go test ./insurance -run TestLiveOpenAI -v
```

This optional command makes billable OpenAI calls with synthetic PDFs and a public plan search. Normal tests use local fixtures and mocked external responses. A live search can legitimately return zero candidates; passing that test does not prove exhaustive plan coverage.

See the [API](../../docs/API.md), [operations guide](../../docs/OPERATIONS.md), [data guide](../DATA_COLLECTION.md), and [architecture](../../ARCHITECTURE.md). Bill upload extraction at `/documents/extract` still returns 501; insurance extraction is a separate implemented flow.

## Supabase and healthcare AI additions

Go now accepts a Supabase bearer token on account/profile, review, PDF and insurance routes. It verifies the token using the configured project's Auth server, requires a confirmed email user, and creates a namespaced local healthcare profile. No legacy account is linked by email. Legacy cookies still require CSRF.

New endpoints: `POST /bills/ai-review`, `GET /care/recommendations`, and implemented `POST /documents/extract`. All three require authenticated identity and explicit AI consent. Context uses the authenticated profile and stored insurance evidence, not plan data supplied by the browser. Missing/invalid OpenAI configuration is reported explicitly; sourced bill rules/catalog data are never presented as completed AI output.

Prefer `npm run backend` from the repository root. It loads public Supabase configuration and inherits the OpenAI key from the launching terminal. `CLEARCARE_ADDR` permits an alternate loopback port. `/health` advertises feature presence and configuration booleans, without returning keys or claiming credential validity.

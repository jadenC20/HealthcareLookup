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

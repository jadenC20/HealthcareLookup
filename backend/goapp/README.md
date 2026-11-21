# Go backend

Run `go run .` from this directory. Go 1.24+ is required. The default listener is `127.0.0.1:8000`; database and data paths are `../clearcare.db` and `../data`.

`CLEARCARE_ADDR` can select another loopback address. The connected frontend uses verified Supabase bearer identities when configured; legacy cookie accounts and CSRF remain supported. `POST /documents/extract`, `POST /bills/ai-review`, and `GET /care/recommendations` add the frontend branch's consent-based healthcare flows alongside the product job APIs.

Product packages are [billreview](billreview/), [caresearch](caresearch/) and [prescription](prescription/). Shared support lives in [account](account/), [insurance](insurance/) and [evidence](evidence/). [main.go](main.go) wires the API. Go is the only backend implementation.

Set `OPENAI_API_KEY` for extraction, explanation and public web search. Without it, deterministic/public API work can still return partial results with cautions. Public NPI Registry and RxNorm calls require network access. Imported reference files and review cohorts are documented in the [data guide](../DATA_COLLECTION.md).

See [API contracts](../../docs/API.md), [operations](../../docs/OPERATIONS.md), [product decisions](../../docs/PRODUCT.md) and [architecture](../../ARCHITECTURE.md).

Long-running product POST routes return persisted job handles. Poll `GET /jobs/{id}` or cancel with `DELETE /jobs/{id}`; completed job responses contain the saved result ID and structured product result. Results and completed jobs are retained for 90 days unless `CLEARCARE_RESULT_RETENTION_DAYS` changes the period.

`go build ./...` checks compilation. Runtime tests for the new product workflows and live external verification are deferred. Existing tests remain runnable with `go test ./...`; opt-in insurance provider checks use `CLEARCARE_LIVE_TEST=1 go test ./insurance -run TestLiveOpenAI -v` and can incur API charges.

# Configuration and operations

## Startup

Use Go 1.24 or later and run `go run .` from `backend/goapp`. The API binds to `127.0.0.1:8000`. The root development launcher starts the API and existing frontend together; this backend change does not update that interface. Relative paths are resolved from the Go process working directory.

Export environment variables through your shell or process manager. [.env.example](../.env.example) documents values; `.env` is not loaded automatically. Never put API keys in browser code or commit them.

| Variable | Default | Purpose |
|---|---|---|
| OPENAI_API_KEY | unset | Enables real card extraction, search, and SBC interpretation |
| OPENAI_MODEL | gpt-4.1 | Structured PDF/image extraction and SBC interpretation |
| OPENAI_SEARCH_MODEL | OPENAI_MODEL | Web search and structured candidate extraction |
| CLEARCARE_PLAN_CATALOG | unset | Optional local CMS/normalized CSV; see [data guide](../backend/DATA_COLLECTION.md) |
| CLEARCARE_DB | ../clearcare.db | SQLite accounts, reviews, cards, insurance workflow and SBC bytes |
| CLEARCARE_DATA_DIR | ../data | Provider and procedure-price CSV directory |
| CLEARCARE_DEMO | unset | `1` selects the care data directory's `demo` subdirectory |
| CLEARCARE_SECURE_COOKIES | unset | `1` enables Secure session cookies; use with HTTPS |
| CLEARCARE_GO | go | Go executable used by the root development launcher |
| CLEARCARE_LIVE_TEST | unset | `1` opts into billable live insurance tests |

Demo mode does **not** disable OpenAI requests. Leave the key unset to exercise explicit unavailable/partial states without external AI calls. A catalog change or environment change requires a backend restart.

## Requests and recovery

Insurance operations are synchronous, with a four-minute workflow deadline and individual OpenAI request timeouts. Configure any future proxy/client timeout accordingly. No background queue or automatic retry scheduler exists. Read `GET /profile/insurance` for the last saved state; a `processing` record older than five minutes is exposed as `interrupted` and can be retried.

Every replacement/action creates a revision. Send the current revision with confirm/select/retry/SBC upload; handle 409 by reloading status. A card replacement resets its derived results. Older calls cannot overwrite a replacement or recreate deleted data. Retry reinterprets a saved SBC when present; otherwise it repeats lookup using confirmed details or the stored card. Correcting details starts a fresh lookup. Reuploading the SBC is also supported.

| Condition | Result/action |
|---|---|
| Missing API key | `openai_not_configured`; configure the key and retry |
| OpenAI 401 | `openai_http_401`; check the running process's credential, then restart/retry |
| OpenAI quota/model/service error | `openai_http_<status>`; inspect project access/billing/service availability; retry after correction |
| No catalog/bad CSV | Catalog caution; web fallback attempted |
| Missing plan attributes | Caution lists the fields; supply corrected plan details |
| Ambiguous/no plan | Select a returned candidate or upload the exact SBC |
| Login page, broken URL, oversize PDF | Retrieval caution; upload a PDF manually |
| Conflicting PDF plan/year | Benefits withheld; select/upload the correct document |
| Missing benefit quote/page | That term is omitted and listed as missing |
| Interrupted operation | Reload status and retry once the processing lease expires |

Do not treat HTTP 200 as proof of complete insurance information: inspect `result.status`, `may_be_inaccurate`, and `cautions`. Partial results and available PDFs are saved even if interpretation fails. Error responses redact remote response bodies; do not add raw card contents, member identifiers, or API credentials to logs.

## Data handling and storage

Uploaded card bytes and downloaded/uploaded SBC PDFs are sent to OpenAI for interpretation. Requests set `store: false`; this setting is not a claim about all provider retention policies. Web search uses extracted public plan attributes rather than the original card, member IDs, or group numbers. Extraction omits member identity fields, but the original stored card may contain them. Do not mistake this storage for deidentified data.

The prototype stores document bytes in SQLite without application-level encryption. Use synthetic data for development and protect database files and backups. Card deletion deletes its insurance workflow/SBC too; deleting insurance results alone preserves the card. Account deletion removes its associated records. These are application deletes, not guaranteed forensic erasure of database pages/backups. Formal HIPAA work remains deferred, as requested.

Stop the server before copying the database for backup. Preserve the backup outside version control. Table creation is additive; there is no migration framework or automated rollback. Catalog/CSV inputs are external files and should be backed up/versioned separately without patient data.

## Verification

From `backend/goapp`:

```sh
go test ./...
go test -race ./...
go vet ./...
CLEARCARE_LIVE_TEST=1 go test ./insurance -run TestLiveOpenAI -v
```

Ordinary tests cover workflow branching, caution persistence, candidate selection, mismatch rejection, source-evidence filtering, unsafe downloads, API revision/CSRF checks, document retry, and user isolation without external AI. The opt-in live suite sends synthetic documents and searches a public plan. It verifies API compatibility, not medical correctness or exhaustive plan discovery. Downloading a real insurer's PDF depends on its availability and access rules.

## API references

Implementation uses the [OpenAI Responses structured-output interface](https://developers.openai.com/api/docs/guides/structured-outputs), [PDF inputs](https://developers.openai.com/api/docs/guides/file-inputs), and [web search with source metadata](https://developers.openai.com/api/docs/guides/tools-web-search). The endpoint is fixed to the official OpenAI API. See [API.md](API.md) for application routes and caution fields.

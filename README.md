# ClearCare

ClearCare's Go backend supports three product workflows: **Bill Review**, **Care Search**, and **Prescription Comparison**. Accounts, insurance import, patient preferences, and private saved results support all three.

| Feature | Backend behavior |
|---|---|
| Bill Review | OpenAI extraction of bills/images and optional EOB; confirmed arithmetic/EOB analysis; contextual imported price benchmarks and NCCI review flags; public reference search; AI explanation; private saved results and separate charge observations |
| Care Search | CMS NPI Registry lookup, local directory fallback, AI web research, source-backed preference matching, optional patient-demographic cohort review matching |
| Prescription Comparison | Public RxNorm product lookup and explicit product selection; AI web search for exact drug/quantity/location prices; source-filtered cash or coupon comparisons |
| Insurance | Card extraction, SBC discovery/selection/download, cited interpretation, manual upload and retry |

These are implemented API flows, not a claim that all external data is available. Missing credentials, reference files, prices, or review demographics produce partial results with cautions. Web-extracted prices and provider attributes need confirmation. Patient demographic reviews are used only when explicitly self-reported aggregate data is supplied; demographics belong to the patient, never the searched doctor.

The frontend has not been updated to expose the new feature APIs. Existing screens may still describe previously unavailable capabilities. No Python backend remains.

## Start

Requires Go 1.24+ and Node.js. Export `OPENAI_API_KEY` in the server environment and run:

```sh
npm run dev -- --demo
```

The frontend runs at `http://127.0.0.1:4173`, the Go API at `http://127.0.0.1:8000`. To run only the backend:

```sh
cd backend/goapp
go run .
```

`CLEARCARE_GO` selects the Go executable for the launcher. Configuration is documented in [.env.example](.env.example); `.env` files are not automatically loaded. Demo mode selects fictional care CSVs; it does not disable real OpenAI/public API requests. OpenAI models default to `gpt-4.1` and can be configured. No additional Go dependencies are required for the public integrations.

## Data and limitations

The default database is `backend/clearcare.db`. `CLEARCARE_DB` and `CLEARCARE_DATA_DIR` override database/reference paths. Product requests run as cancellable background jobs and successful results are saved privately. Completed results/jobs are retained for 90 days by default; `CLEARCARE_RESULT_RETENTION_DAYS` configures 1–3650 days. Patient preferences and saved results are private account data. Charge observation rows exclude patient IDs, names, claim numbers and dates; a separate ownership table provides private access. This linkage means the database as a whole is not anonymous.

Imports are optional enhancements. Bill Review falls back to quoted public-source rates, then bill/EOB checks and billing questions when no applicable price is found. Public fallback rates are explicitly cautioned and excluded from charge observations. Care Search continues general matching without demographic-review data. The app does not automatically ingest arbitrary large hospital/TiC files. RxNorm identifies medications and does not supply retail prices. Public drug-price offers can be absent, outdated or conditional. NPI registration does not verify network participation, availability or clinical quality.

## Documentation

- [Product behavior and decisions](docs/PRODUCT.md)
- [Architecture and backend/frontend graphs](ARCHITECTURE.md)
- [API contracts](docs/API.md)
- [Operations and configuration](docs/OPERATIONS.md)
- [Reference imports and public sources](backend/DATA_COLLECTION.md)

## Verification

A Go build checks compilation. Tests and live endpoint checks for the new product flows are deferred at the user's request. Existing tests remain available:

```sh
cd backend/goapp
go test ./...
go vet ./...
```

The opt-in `CLEARCARE_LIVE_TEST=1 go test ./insurance -run TestLiveOpenAI -v` makes paid OpenAI calls with synthetic documents. The last live check returned 401 with the configured credential; live success is not verified.

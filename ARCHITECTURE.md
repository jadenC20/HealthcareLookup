# ClearCare architecture

The active server is a single Go application in [backend/goapp](backend/goapp). The product features are **Bill Review, Care Search, and Prescription Comparison**. Accounts and insurance-plan import are shared capabilities. These packages are modules in one process, not separately deployed microservices.

## Backend graph

```mermaid
flowchart TD
    Client[Existing frontend or API client] --> Proxy[Node API proxy]
    Proxy --> Router[Go HTTP router and request limits]
    Router --> Account[account: identity, profile, reviews and reports]
    Router --> Bills[billreview: deterministic bill and EOB checks]
    Router --> Care[caresearch: provider and procedure-price CSV search]
    Account --> Insurance[insurance: card to SBC workflow]
    Insurance --> AI[OpenAI Responses: extraction and interpretation]
    Insurance --> Catalog[Optional CMS plan catalogue CSV]
    Insurance --> Search[OpenAI hosted web search]
    Search --> Candidates[Source-backed plan candidates]
    Catalog --> Candidates
    Candidates --> Match[Exact variant or user selection]
    Match --> Fetch[Bounded public HTTPS PDF retrieval]
    Fetch --> AI
    Insurance --> Result[Benefits, source hash, page citations and cautions]
    Account --> DB[(SQLite)]
    Result --> Account
    Account --> PDF[Review and account PDF reports]
    Care --> CSV[Provider and service-price CSVs]
    Prescription[Prescription Comparison: not implemented]
```

## Frontend graph: existing code, unchanged in this iteration

```mermaid
flowchart TD
    Shell[index.html and styles.css] --> App[app.js navigation and dashboard]
    App --> Integration[integration.js]
    Integration --> BillUI[Manual bill and EOB editor]
    Integration --> CareUI[Provider and price search]
    Integration --> Profile[Accounts and insurance-card upload]
    Integration --> Meds[Prescription unavailable notice]
    BillUI & CareUI & Profile --> Proxy[server.mjs static server and API proxy]
    Proxy --> Go[Go API]
    Future[Future UI: candidates, corrections, SBC upload and cautions] -.-> Go
```

The frontend's existing card copy still describes lookup as unavailable. The backend now runs it, but this iteration deliberately does not update frontend files. The complete workflow can be used through the [API](docs/API.md).

## Insurance processing sequence

```mermaid
sequenceDiagram
    participant User as API client
    participant API as Account API
    participant DB as SQLite
    participant Engine as Insurance engine
    participant AI as OpenAI
    participant Source as CMS catalogue / public document
    User->>API: Authenticated card upload + CSRF
    API->>DB: Save card and new processing revision
    API->>Engine: Start bounded synchronous workflow
    Engine->>AI: Extract card plan fields
    Engine->>Source: Search configured plan catalogue
    opt No unique exact catalogue match
        Engine->>AI: Public plan search with sources
    end
    Engine-->>API: Candidates and cautions
    API->>DB: Persist only if revision still current
    API-->>User: Result + revision
    User->>API: Confirm details or select candidate
    API->>Engine: Continue with new guarded revision
    Engine->>Source: Retrieve selected public PDF
    Engine->>AI: Interpret PDF with page quotes
    Engine-->>API: Benefits + source metadata + missing steps
    API->>DB: Save result and document if still current
    API-->>User: Ready-with-caution or partial result
```

Confirmed details plus one exact CMS plan variant/year/state match can proceed through retrieval and interpretation without candidate selection. Extracted-but-unconfirmed details and web candidates require confirmation. Manual SBC upload enters at the document interpretation stage.

## Modules and boundaries

| Module | Responsibility |
|---|---|
| [main.go](backend/goapp/main.go) | HTTP route registration, loopback listener, origin policy, actual request-body limit |
| [account/api.go](backend/goapp/account/api.go) | Sessions, CSRF, account/profile/review endpoints, card storage, reports |
| [account/insurance.go](backend/goapp/account/insurance.go) | Insurance endpoints, persistence, revision guards, four-minute workflow deadline |
| [insurance/workflow.go](backend/goapp/insurance/workflow.go) | Stage orchestration, deterministic matching, cautions, evidence acceptance |
| [insurance/openai.go](backend/goapp/insurance/openai.go) | Responses API, strict JSON schemas, PDF/image input, sourced web search |
| [insurance/catalog.go](backend/goapp/insurance/catalog.go) | Optional local CMS/normalized CSV catalogue |
| [insurance/fetch.go](backend/goapp/insurance/fetch.go) | Public HTTPS PDF retrieval, redirect/DNS validation and size/time limits |
| [billreview](backend/goapp/billreview) | Existing user-confirmed bill arithmetic, duplicates, vague charges, EOB checks |
| [caresearch](backend/goapp/caresearch) | Existing CSV validation, directory and price queries |

No framework or Python runtime is used by the active server. SQLite, scrypt and PDF generation use pinned Go dependencies. The historical Python packages remain for reference and are not started by [dev.mjs](dev.mjs).

## Persistence and concurrency

SQLite tables: `users`, `sessions`, `reviews`, `insurance_cards`, and `insurance_workflows`. Workflow rows contain an opaque revision, JSON result, and optional SBC bytes. A new card replaces the previous card and invalidates its derived result. User actions require the latest revision. A late external response cannot resurrect deleted data or overwrite a replacement card. Account deletion removes cards and workflow rows in the account transaction.

The workflow is synchronous, bounded by four minutes. GET can inspect persisted processing state during a request. An abandoned processing row older than five minutes is reported as interrupted and is retryable. This is not a durable background job queue; a server restart interrupts in-flight work. Retrying does not resume an OpenAI conversation. There is no hidden retry loop that can repeatedly incur charges.

## External data and limits

Card/SBC bytes are sent to OpenAI; search receives only plan fields, not member identifiers or the card file. The app requests `store: false`. The server fetches public PDF sources with a 10 MB cap and 45-second HTTP timeout. OpenAI calls have 90-second timeouts. Raw external errors are reduced to stable caution codes without echoing documents or credentials.

SBC results describe plan terms. They do not establish active membership, remaining deductible, network participation, or authorization for care. Citations are AI-extracted page references and quotes, retained with the source/hash for human inspection. They are not independently verified by a second PDF parser.

See [operations](docs/OPERATIONS.md), [API contract](docs/API.md), and [product specification](docs/PRODUCT.md) for details and remaining work.

# ClearCare architecture

**Rewrite status:** A partial Go implementation is being developed in `backend/goapp/`. It is not wired into the application and does not yet support accounts, document extraction, saved reviews, or PDF exports. The diagrams below describe the existing Python application, not the incomplete Go implementation.

ClearCare is a local-first prototype. The browser talks to a loopback Node server, which serves the frontend and proxies API calls to FastAPI. The API is organized into bill review, care search, and account services. Prescription pricing is presented as unavailable; it has no backend integration.

## Frontend graph

```mermaid
flowchart TD
    User[User] --> Shell[index.html and styles.css]
    Shell --> App[app.js: navigation and dashboard]
    App --> Bill[Bill review and extraction editor]
    App --> Care[Provider and price search]
    App --> Meds[Prescription availability notice]
    App --> Account[Sign in, profile, saved reviews]
    Bill & Care & Account --> Integration[integration.js: API calls and rendering]
    Integration --> Node[server.mjs: static files and /api proxy]
    Node --> API[FastAPI on loopback]
```

The frontend is vanilla JavaScript. `integration.js` adds API-backed screens to the shell and holds the active review in browser memory. No health data is stored in localStorage. The Node server serves an explicit asset allowlist and forwards `/api/` requests to port 8000.

## Backend graph

```mermaid
flowchart TD
    Proxy[Node /api proxy] --> Main[main.py: FastAPI, middleware, lifespan]
    Main --> BillAPI[bill_review/api.py]
    Main --> CareAPI[care_search/api.py]
    Main --> AccountAPI[account/api.py]
    BillAPI --> Extract[documents.py: PDF, image and OCR extraction]
    Extract --> OCR[Native Tesseract or Node WASM OCR]
    BillAPI --> Rules[models, checks and inquiry]
    BillAPI --> Report[reports.py: PDF export]
    AccountAPI --> Report
    BillAPI & AccountAPI --> Auth[account/service.py: sessions and CSRF]
    CareAPI --> Loader[care_search/loader.py]
    CSV[Provider and price CSVs] --> Loader
    Loader & CareAPI & BillAPI & AccountAPI & Auth --> DB[(SQLite via database.py)]
```

`dev.mjs` starts both servers on loopback. `main.py` composes the three routers, applies request middleware and CORS, initializes account tables, and loads reference data at startup. The routers retain the existing URLs:

| Service | Responsibilities | API paths |
|---|---|---|
| Bill review | Extract documents, analyze reviewed fields, manage saved reviews, generate review PDFs | `/sample`, `/documents/extract`, `/bills/analyze`, `/reviews`, `/reviews/{id}`, `/reviews/{id}/pdf`, `/reports/pdf` |
| Care search | Load CSV reference data and query providers and prices | `/providers`, `/prices/compare`, `/data/status`, `/data/reload` (disabled) |
| Account | Registration, sessions, profile, account deletion and account PDF | `/auth/*`, `/profile`, `/account`, `/account/pdf` |
| Shared application | Health, middleware and SQLite connection/schema helpers | `/health` |

The account PDF uses bill review's report generator and saved-review records. Account deletion also deletes saved reviews. These are explicit cross-feature dependencies; the three folders are modules in one application, not independently deployed services.

## Data and trust boundaries

A signed-in user uploads a bill and optional EOB for local extraction. Text and suggestions return to the browser for review; original files and raw extraction text are not saved. Confirmed fields are analyzed by deterministic rules. Saving a review stores its structured request and findings in SQLite. Reports are generated on request.

Provider and price CSVs are imported at startup; demo mode uses fictional data. SQLite also holds accounts, hashed session tokens, CSRF tokens and saved reviews. Its default path is `backend/clearcare.db`, configurable with `CLEARCARE_DB`. Passwords use scrypt; the cookie is HttpOnly and SameSite Strict, with optional Secure. Authenticated mutations check CSRF. The database is not application-encrypted. There is no email recovery, coverage verification, medication feed, or cloud extraction service. See the root README for operational limitations.

## Verification

Run `npm run check` for JavaScript syntax and `.venv/bin/python -m pytest backend/tests -q` for Python tests. OCR tests require a local OCR engine or the downloaded model and Node dependencies. Python dependencies are pinned in `backend/requirements-lock.txt`; Node dependencies are pinned in `package-lock.json`.

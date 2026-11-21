# ClearCare

Local frontend and FastAPI backend for HealthcareLookup. Documents are processed on this machine; no cloud extraction or AI service is used.

## Run

Requires Node.js 20+, Python 3.10+, and a local Python virtual environment.

```powershell
python -m venv .venv
.\.venv\Scripts\python.exe -m pip install -r backend/requirements-lock.txt
npm ci
node backend/setup-ocr.mjs
npm run dev -- --demo
```

On macOS/Linux use `.venv/bin/python` for pip. The OCR setup downloads the English model once; document processing then works locally. Native Tesseract is optional (set `TESSERACT_CMD`); the Node/WASM fallback works without a system installation.

Open http://127.0.0.1:4173. FastAPI runs on port 8000; the frontend proxies `/api` to it, including session cookies. Use `npm run dev` without `--demo` to load your own `backend/data/providers.csv` and `backend/data/service_prices.csv`. Demo mode uses explicitly fictional reference prices. Do not run a second server on the same ports.

Checks: `npm run check` and `.\.venv\Scripts\python.exe -m pytest backend/tests -q`. All tests use temporary databases. `requirements-lock.txt` records the tested Python dependencies.

## Implemented

- Responsive overview and navigation.
- Local PDF/PNG/JPEG extraction with text PDFs and OCR for scans, 10 MB/file, 12 pages/PDF, and 20 megapixel images. Extracted bill suggestions are best-effort. EOB line columns require manual confirmation. A user must review editable fields before analysis.
- API-backed bill checks, findings, and inquiry drafts; original files are discarded after processing.
- Account registration, sign in/out, profile editing, private saved reviews, record deletion, and account deletion. Passwords use scrypt; sessions are hashed server-side and expire after 12 hours. Mutations require CSRF tokens; authentication attempts are throttled.
- PDF export of the current review, saved review, or profile plus all saved reviews. No CSV/JSON export UI.
- API-backed provider directory and procedure price comparisons with provenance and warnings. Medication pricing is not connected and is shown as unavailable.
- Keyboard focus indicators and native dialog behavior.

## Data and integration boundaries

The sample bill and demo CSVs are fictional. With custom CSVs the UI shows their source, retrieval date, and demo markers. No insurance coverage verification or medication pricing feed is connected. Extraction is local OCR and conservative pattern matching, not an AI model. The reviewed fields drive deterministic analysis. Do not commit real patient documents, database files, or credentials.

Saved data resides in `backend/clearcare.db` (override with `CLEARCARE_DB`). No health data is stored in browser localStorage. Original uploads and full OCR text are not retained. Exports include reviewed line items and evidence, not original documents. Account PDF exports support western text; unsupported glyphs are replaced with `?` in this prototype.

This is a loopback-only hackathon application, not a production patient portal. Database contents are not application-encrypted. There is no email verification, password recovery, or multi-device account service. Public deployment requires HTTPS, `CLEARCARE_SECURE_COOKIES=1`, an explicit production-origin policy, protected storage/backups, operational review, and an account-recovery design. Do not expose these local development servers publicly.

## Project layout

- `index.html`: application shell and navigation
- `styles.css`: responsive design system
- `app.js`: views, fictional datasets, and browser interactions
- `integration.js`: API-backed screens, account forms, extraction editor, saved records, PDF downloads
- `server.mjs`: loopback-only static server and backend proxy
- `dev.mjs`: starts frontend and backend together
- `backend/`: document extraction/OCR, accounts, SQLite storage, API, checks, reports, tests

No external fonts, analytics, or cloud document services are used. The one-time package/model installation needs internet access.

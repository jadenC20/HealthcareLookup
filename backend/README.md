# ClearCare backend

FastAPI + SQLite backend with local document extraction/OCR, accounts, saved reviews, and PDF reports. The bill checks are plain Python rules. See the root README for the combined startup and OCR installation instructions.

## Run it

```
pip install -r requirements.txt
uvicorn main:app --reload
```

Open http://localhost:8000/docs to try the endpoints.

## Endpoints

| Endpoint | What it does |
|---|---|
| POST /bills/analyze | Takes a bill (and optional EOB) as JSON. Returns flags with evidence and a draft billing inquiry. |
| GET /providers | Lists providers. Optional `city` and `specialty`. |
| GET /prices/compare | Needs `procedure_code`. Optional `city` and `price_type`. Returns prices plus warnings when results are not comparable. |
| GET /data/status | Shows how many rows loaded and any CSV errors. |
| POST /data/reload | Disabled; restart the backend to reload CSV files. |

Try the analyze endpoint with `sample/bill_with_problems.json`.

## Checks in bill_review/checks.py

- Possible duplicate charge
- Line items do not match the subtotal
- Unclear fee description
- Bill and EOB show different billed amounts
- Bill and EOB show different patient amounts
- EOB line does not add up

These are flags to confirm, not proof of an error.

## Data

The server loads `data/providers.csv` and `data/service_prices.csv` on startup. See `DATA_COLLECTION.md`. Fake demo rows are in `data/demo/`.

## Tests

```
python -m pytest
```

## Document and account endpoints

| Endpoint | Purpose |
|---|---|
| POST /auth/register, /auth/login, /auth/logout | Local account sessions |
| GET /auth/me | Restore signed-in profile and CSRF token |
| PATCH /profile | Update name, city, and optional plan |
| POST /documents/extract | Authenticated multipart upload: `file` and `kind` (`bill` or `eob`) |
| POST /reviews | Save confirmed structured input and freshly computed findings |
| GET /reviews, /reviews/{id} | List or reopen only the current user's reviews |
| DELETE /reviews/{id} | Delete an owned review |
| POST /reports/pdf | Export a current structured review as PDF |
| GET /reviews/{id}/pdf, /account/pdf | Export owned records as PDF |
| DELETE /account | Delete the signed-in account, sessions, and saved reviews |

Authenticated mutations require the `X-CSRF-Token` returned by login/me. Cookies are HttpOnly, SameSite=Strict, and expire after 12 hours. Local HTTP is for loopback development only. Data reload over HTTP is disabled; restart after changing CSVs.

PDFs, PNGs, and JPEGs are validated and processed locally. PDFs use their text layer or OCR. Encrypted PDFs are rejected. Limits: 10 MB per file, 12 PDF pages, 20 megapixel images. Structured bill suggestions use conservative patterns; arbitrary layouts can require manual line entry. EOB line columns must be reviewed/entered manually. Missing amounts are not assumed to be zero. Original documents and raw extracted text are not saved.

## Remaining integrations

- Medication prices
- Verified insurer/network feeds
- Email verification and password recovery for public deployment
- Original-document retention (intentionally not implemented)

## Code layout

`main.py` composes the `bill_review`, `care_search`, and `account` routers. Each feature package owns its API routes; `database.py` provides shared SQLite access. Prescription pricing is not connected and has no backend service. API paths remain unchanged. See the root `ARCHITECTURE.md` for dependency graphs.

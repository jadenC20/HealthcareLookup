# ClearCare backend

Small FastAPI + SQLite backend. No AI model yet. The bill checks are plain Python rules, so they are easy to test and explain.

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
| POST /data/reload | Reloads the CSV files after you edit them. |

Try the analyze endpoint with `sample/bill_with_problems.json`.

## Checks in checks.py

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

## Not built yet

- Uploading PDFs and pulling out the fields (right now the bill comes in as JSON)
- Medication prices
- Saving uploaded documents (on purpose, so nothing private is stored)

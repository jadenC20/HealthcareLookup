# Data collection guide (your part)

Goal for tonight is small and real. Pick one city, three procedures, and 4 to 6 places per procedure. That is about 15 price rows.

Suggested city is Philadelphia. Suggested procedures are below. Check each code against the hospital's own file before you use it, because hospitals sometimes list codes differently.

| Procedure | Code |
|---|---|
| MRI brain without contrast | 70551 |
| MRI lower leg joint (knee) without contrast | 73721 |
| CT abdomen and pelvis with contrast | 74177 |

## Step 1. Fill in providers.csv

Copy `data/templates/providers.csv` to `data/providers.csv` and add one row per place.

- Find hospitals and imaging centers by searching "[hospital name] price transparency" or "standard charges".
- For the NPI number, search the NPI Registry (npiregistry.cms.hhs.gov) by name and city.
- `provider_type` is `facility` for hospitals and imaging centers.
- `source_url` is the page where you found the provider info. `retrieved_date` is today, like 2026-11-20.
- Give each place a short unique `provider_id`, like `penn1` or `temple1`.

## Step 2. Fill in service_prices.csv

Copy `data/templates/service_prices.csv` to `data/service_prices.csv`.

1. Download the hospital's price file (usually a big CSV or JSON on its price transparency page).
2. Search the file for the procedure code.
3. Add one row per price you want to show.

| Column | What to put |
|---|---|
| provider_id | Same id you used in providers.csv |
| procedure_code | Code from the table above |
| description | Description from the hospital file |
| price | Number only, like 1250.00 |
| price_type | `gross_charge`, `discounted_cash`, `payer_negotiated`, or `other` |
| payer_name, plan_name | Only for `payer_negotiated`. Plan name is required then. |
| includes | What the price covers, like "facility fee only". Leave blank if the file does not say. |
| insurance_status | `unknown` unless you checked the insurer's directory. Then `confirmed` or `unverified`. |
| source_url | Link to the price file or page |
| retrieved_date | Today's date, like 2026-11-20 |
| notes | Anything odd, like "price covers imaging only" |

Rules to keep the data honest.

- Do not mix price types in your head. Gross charge is a list price. Discounted cash is what you pay without insurance. Negotiated is what one insurer pays.
- Do not guess a price or an inclusion. Leave `includes` blank if unsure. The app will warn users.
- If you cannot find a real price, skip that row. Do not make one up.
- Rows with `DEMO` in source_url are treated as fake and labeled that way in the app.

## Step 3. Check your files

```
python loader.py data/providers.csv data/service_prices.csv
```

It prints how many rows loaded and lists every row that needs fixing, with the row number.

If the server is running, you can also call `POST /data/reload` and then `GET /data/status`.

## Also useful for the demo

Make one synthetic bill and EOB for your own sample. Copy `sample/bill_with_problems.json` and change the names and amounts. Do not use anyone's real bill.

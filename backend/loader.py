"""
Loads the CSV files you collect into the database and tells you what is wrong.

Run it by hand to check your data:
    python loader.py data/providers.csv data/service_prices.csv
"""
import csv
import os
import sys
from datetime import date

import database

PRICE_TYPES = ["gross_charge", "discounted_cash", "payer_negotiated", "other"]
INSURANCE_STATUSES = ["confirmed", "unverified", "unknown"]

PROVIDER_FIELDS = ["provider_id", "name", "npi", "provider_type", "specialty", "address",
                   "city", "state", "zip", "phone", "website", "source_url", "retrieved_date"]
PRICE_FIELDS = ["provider_id", "procedure_code", "description", "price", "price_type",
                "payer_name", "plan_name", "includes", "insurance_status",
                "source_url", "retrieved_date", "notes"]


def is_demo(source_url):
    return 1 if source_url.strip().upper().startswith("DEMO") else 0


def good_date(text):
    try:
        date.fromisoformat(text)
        return True
    except ValueError:
        return False


def read_csv(path):
    with open(path, newline="", encoding="utf-8-sig") as f:
        return list(csv.DictReader(f))


def load_providers(conn, path):
    loaded = 0
    errors = []
    for i, row in enumerate(read_csv(path), start=2):  # row 1 is the header
        row = {k: (v or "").strip() for k, v in row.items() if k}
        if not row.get("provider_id") or not row.get("name"):
            errors.append(f"{os.path.basename(path)} row {i}: provider_id and name are required")
            continue
        if not row.get("source_url"):
            errors.append(f"{os.path.basename(path)} row {i}: source_url is required")
            continue
        if not good_date(row.get("retrieved_date", "")):
            errors.append(f"{os.path.basename(path)} row {i}: retrieved_date must look like 2026-11-20")
            continue
        values = [row.get(f, "") for f in PROVIDER_FIELDS] + [is_demo(row["source_url"])]
        conn.execute(
            "INSERT OR REPLACE INTO providers (" + ",".join(PROVIDER_FIELDS) + ",is_demo) "
            "VALUES (" + ",".join("?" * (len(PROVIDER_FIELDS) + 1)) + ")",
            values,
        )
        loaded += 1
    conn.commit()
    return loaded, errors


def load_prices(conn, path):
    loaded = 0
    errors = []
    known = set(r["provider_id"] for r in conn.execute("SELECT provider_id FROM providers"))
    name = os.path.basename(path)
    for i, row in enumerate(read_csv(path), start=2):
        row = {k: (v or "").strip() for k, v in row.items() if k}
        if row.get("provider_id") not in known:
            errors.append(f"{name} row {i}: provider_id '{row.get('provider_id')}' is not in providers.csv")
            continue
        if not row.get("procedure_code"):
            errors.append(f"{name} row {i}: procedure_code is required")
            continue
        try:
            price = float(row.get("price", "").replace("$", "").replace(",", ""))
            if price < 0:
                raise ValueError
        except ValueError:
            errors.append(f"{name} row {i}: price must be a number like 1250.00")
            continue
        if row.get("price_type") not in PRICE_TYPES:
            errors.append(f"{name} row {i}: price_type must be one of {', '.join(PRICE_TYPES)}")
            continue
        if row["price_type"] == "payer_negotiated" and not row.get("plan_name"):
            errors.append(f"{name} row {i}: payer_negotiated prices need a plan_name")
            continue
        if not row.get("source_url"):
            errors.append(f"{name} row {i}: source_url is required")
            continue
        if not good_date(row.get("retrieved_date", "")):
            errors.append(f"{name} row {i}: retrieved_date must look like 2026-11-20")
            continue
        status = row.get("insurance_status") or "unknown"
        if status not in INSURANCE_STATUSES:
            errors.append(f"{name} row {i}: insurance_status must be confirmed, unverified, or unknown")
            continue
        row["insurance_status"] = status
        row["price"] = price
        values = [row.get(f, "") for f in PRICE_FIELDS] + [is_demo(row["source_url"])]
        conn.execute(
            "INSERT INTO service_prices (" + ",".join(PRICE_FIELDS) + ",is_demo) "
            "VALUES (" + ",".join("?" * (len(PRICE_FIELDS) + 1)) + ")",
            values,
        )
        loaded += 1
    conn.commit()
    return loaded, errors


def load_all(conn, providers_path, prices_path):
    database.clear_data(conn)
    result = {"providers_loaded": 0, "prices_loaded": 0, "errors": []}
    if os.path.exists(providers_path):
        n, errs = load_providers(conn, providers_path)
        result["providers_loaded"] = n
        result["errors"] += errs
    else:
        result["errors"].append(f"{providers_path} not found")
    if os.path.exists(prices_path):
        n, errs = load_prices(conn, prices_path)
        result["prices_loaded"] = n
        result["errors"] += errs
    else:
        result["errors"].append(f"{prices_path} not found")
    return result


if __name__ == "__main__":
    providers_file = sys.argv[1] if len(sys.argv) > 1 else "data/providers.csv"
    prices_file = sys.argv[2] if len(sys.argv) > 2 else "data/service_prices.csv"
    conn = database.get_connection()
    database.init_db(conn)
    result = load_all(conn, providers_file, prices_file)
    print(f"Providers loaded: {result['providers_loaded']}")
    print(f"Prices loaded: {result['prices_loaded']}")
    if result["errors"]:
        print("\nFix these rows:")
        for e in result["errors"]:
            print(" -", e)
    else:
        print("No problems found.")

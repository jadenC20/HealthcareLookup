from contextlib import asynccontextmanager
from typing import Optional

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

import database
import loader
from checks import run_all_checks
from inquiry import build_inquiry
from models import AnalyzeRequest

PROVIDERS_CSV = "data/providers.csv"
PRICES_CSV = "data/service_prices.csv"

load_status = {"providers_loaded": 0, "prices_loaded": 0, "errors": []}


def reload_data():
    conn = database.get_connection()
    database.init_db(conn)
    result = loader.load_all(conn, PROVIDERS_CSV, PRICES_CSV)
    conn.close()
    load_status.update(result)
    return result


@asynccontextmanager
async def lifespan(app):
    reload_data()
    yield


app = FastAPI(title="ClearCare API", lifespan=lifespan)

app.add_middleware(
    CORSMiddleware,
    allow_origins=["http://localhost:3000"],
    allow_methods=["*"],
    allow_headers=["*"],
)


@app.get("/health")
def health():
    return {"status": "ok"}


@app.post("/bills/analyze")
def analyze_bill(request: AnalyzeRequest):
    findings = run_all_checks(request.bill, request.eob)
    inquiry = build_inquiry(
        request.bill.provider_name,
        request.bill.claim_number,
        findings,
        request.patient_name,
        request.account_number,
    )
    return {
        "flag_count": len(findings),
        "findings": findings,
        "inquiry_draft": inquiry,
        "note": "Flags are things to confirm, not proof of an error.",
    }


@app.get("/providers")
def get_providers(city: Optional[str] = None, specialty: Optional[str] = None):
    conn = database.get_connection()
    results = database.search_providers(conn, city, specialty)
    conn.close()
    return {"results": results}


@app.get("/prices/compare")
def compare_prices(procedure_code: str, city: Optional[str] = None,
                   price_type: Optional[str] = None):
    conn = database.get_connection()
    data = database.compare_prices(conn, procedure_code, city, price_type)
    conn.close()
    return data


@app.get("/data/status")
def data_status():
    return load_status


@app.post("/data/reload")
def data_reload():
    # Call this after you edit the CSV files
    return reload_data()

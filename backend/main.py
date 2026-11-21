from contextlib import asynccontextmanager
from typing import Optional

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

import database
import loader
from checks import run_all_checks
from inquiry import build_inquiry
from models import AnalyzeRequest
import os
from pathlib import Path
import accounts
from user_api import router
from fastapi import Request
from fastapi.responses import JSONResponse

BASE = Path(__file__).resolve().parent
PROVIDERS_CSV = str(BASE / ('data/demo/providers.csv' if os.getenv('CLEARCARE_DEMO') == '1' else 'data/providers.csv'))
PRICES_CSV = str(BASE / ('data/demo/service_prices.csv' if os.getenv('CLEARCARE_DEMO') == '1' else 'data/service_prices.csv'))

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
    accounts.init()
    yield


app = FastAPI(title="ClearCare API", lifespan=lifespan)
app.include_router(router)

@app.middleware('http')
async def protect_requests(request: Request, call_next):
    allowed = {'http://localhost:4173','http://127.0.0.1:4173','http://localhost:8000','http://127.0.0.1:8000'}
    if request.method not in ('GET','HEAD','OPTIONS') and request.headers.get('origin') and request.headers['origin'] not in allowed:
        return JSONResponse({'detail':'Origin not allowed.'},status_code=403)
    try:
        size=int(request.headers.get('content-length','0'))
    except ValueError:
        return JSONResponse({'detail':'Invalid request size.'},status_code=400)
    if size > 11*1024*1024:
        return JSONResponse({'detail':'Request exceeds 11 MB.'},status_code=413)
    response=await call_next(request)
    response.headers['Cache-Control']='no-store'
    response.headers['X-Content-Type-Options']='nosniff'
    return response

app.add_middleware(
    CORSMiddleware,
    allow_origins=["http://localhost:4173", "http://127.0.0.1:4173"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)


@app.get("/health")
def health():
    return {"status": "ok"}

@app.get('/sample')
def sample():
    import json
    return json.loads((BASE / 'sample/bill_with_problems.json').read_text())


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
def data_reload(request: Request):
    # Call this after you edit the CSV files
    from fastapi import HTTPException
    raise HTTPException(403, 'Reload data by restarting the local backend.')

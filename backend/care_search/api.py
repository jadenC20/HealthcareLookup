import os
from pathlib import Path
from typing import Optional
from fastapi import APIRouter, Request
import database
from care_search import loader

router = APIRouter()
BASE = Path(__file__).resolve().parent.parent
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

@router.get("/providers")
def get_providers(city: Optional[str] = None, specialty: Optional[str] = None):
    conn = database.get_connection()
    results = database.search_providers(conn, city, specialty)
    conn.close()
    return {"results": results}


@router.get("/prices/compare")
def compare_prices(procedure_code: str, city: Optional[str] = None,
                   price_type: Optional[str] = None):
    conn = database.get_connection()
    data = database.compare_prices(conn, procedure_code, city, price_type)
    conn.close()
    return data


@router.get("/data/status")
def data_status():
    return load_status


@router.post("/data/reload")
def data_reload(request: Request):
    # Reference data is loaded at startup, not through HTTP.
    from fastapi import HTTPException
    raise HTTPException(403, 'Reload data by restarting the local backend.')

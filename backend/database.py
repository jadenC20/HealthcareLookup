import os
import sqlite3
from pathlib import Path

DB_PATH = os.environ.get("CLEARCARE_DB", str(Path(__file__).resolve().parent / "clearcare.db"))


def get_connection():
    conn = sqlite3.connect(DB_PATH)
    conn.row_factory = sqlite3.Row
    return conn


def init_db(conn):
    conn.execute("""
        CREATE TABLE IF NOT EXISTS providers (
            provider_id TEXT PRIMARY KEY,
            name TEXT NOT NULL,
            npi TEXT,
            provider_type TEXT,
            specialty TEXT,
            address TEXT,
            city TEXT,
            state TEXT,
            zip TEXT,
            phone TEXT,
            website TEXT,
            source_url TEXT NOT NULL,
            retrieved_date TEXT NOT NULL,
            is_demo INTEGER DEFAULT 0
        )
    """)
    conn.execute("""
        CREATE TABLE IF NOT EXISTS service_prices (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            provider_id TEXT NOT NULL,
            procedure_code TEXT NOT NULL,
            description TEXT,
            price REAL NOT NULL,
            price_type TEXT NOT NULL,
            payer_name TEXT,
            plan_name TEXT,
            includes TEXT,
            insurance_status TEXT DEFAULT 'unknown',
            source_url TEXT NOT NULL,
            retrieved_date TEXT NOT NULL,
            notes TEXT,
            is_demo INTEGER DEFAULT 0
        )
    """)
    conn.commit()


def clear_data(conn):
    conn.execute("DELETE FROM service_prices")
    conn.execute("DELETE FROM providers")
    conn.commit()


def search_providers(conn, city=None, specialty=None):
    sql = "SELECT * FROM providers WHERE 1=1"
    params = []
    if city:
        sql += " AND lower(city) = lower(?)"
        params.append(city)
    if specialty:
        sql += " AND lower(specialty) LIKE lower(?)"
        params.append("%" + specialty + "%")
    sql += " ORDER BY name"
    return [dict(r) for r in conn.execute(sql, params).fetchall()]


def compare_prices(conn, procedure_code, city=None, price_type=None):
    sql = """
        SELECT p.provider_id, p.name, p.address, p.city, p.state, p.phone, p.website,
               s.procedure_code, s.description, s.price, s.price_type, s.payer_name,
               s.plan_name, s.includes, s.insurance_status, s.source_url,
               s.retrieved_date, s.notes, s.is_demo
        FROM service_prices s
        JOIN providers p ON p.provider_id = s.provider_id
        WHERE s.procedure_code = ?
    """
    params = [procedure_code]
    if city:
        sql += " AND lower(p.city) = lower(?)"
        params.append(city)
    if price_type:
        sql += " AND s.price_type = ?"
        params.append(price_type)
    sql += " ORDER BY s.price_type, s.price"
    results = [dict(r) for r in conn.execute(sql, params).fetchall()]

    # Warnings so the user does not compare things that are not comparable
    warnings = []
    if len(set(r["price_type"] for r in results)) > 1:
        warnings.append("Results mix different price types. Only compare prices of the same type.")
    if any(not r["includes"] for r in results):
        warnings.append("Some results do not say what the price includes.")
    includes_set = set(r["includes"] for r in results if r["includes"])
    if len(includes_set) > 1:
        warnings.append("Prices include different things (for example facility fee only vs. "
                        "facility plus doctor's reading). Check before comparing.")
    if any(r["is_demo"] for r in results):
        warnings.append("Some results are demonstration data, not real prices.")
    return {"results": results, "warnings": warnings}

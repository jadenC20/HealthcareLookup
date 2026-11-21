import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.dirname(__file__)))

from bill_review.models import Bill, EOB
from bill_review.checks import run_all_checks

SAMPLE = os.path.join(os.path.dirname(os.path.dirname(__file__)), "sample", "bill_with_problems.json")


def load_sample():
    with open(SAMPLE) as f:
        data = json.load(f)
    return Bill(**data["bill"]), EOB(**data["eob"]), data


def rules(findings):
    return [f.rule for f in findings]


def test_clean_bill_has_no_flags():
    bill, eob, _ = load_sample()
    # Remove the duplicate line and fix the totals
    bill.lines = [bill.lines[0], bill.lines[2]]
    bill.lines[1].description = "Contrast agent administration"
    eob.lines[1].description = "Contrast agent administration"
    bill.stated_subtotal = 1600.00
    bill.patient_responsibility = 320.00
    assert run_all_checks(bill, eob) == []


def test_duplicate_is_flagged():
    bill, eob, _ = load_sample()
    assert "possible_duplicate" in rules(run_all_checks(bill, eob))


def test_same_code_different_description_is_not_a_duplicate():
    bill, _, _ = load_sample()
    bill.lines[1].description = "MRI lower extremity joint without contrast, second side"
    bill.stated_subtotal = 3050.00
    assert "possible_duplicate" not in rules(run_all_checks(bill))


def test_duplicate_wording_does_not_say_error():
    bill, _, _ = load_sample()
    for f in run_all_checks(bill):
        if f.rule == "possible_duplicate":
            assert "error" not in f.message.lower()
            assert "confirm" in f.message.lower()


def test_bill_eob_mismatch_found():
    bill, eob, _ = load_sample()
    found = rules(run_all_checks(bill, eob))
    assert "billed_amount_mismatch" in found
    assert "patient_responsibility_mismatch" in found


def test_subtotal_mismatch():
    bill, _, _ = load_sample()
    bill.stated_subtotal = 2000.00
    assert "subtotal_mismatch" in rules(run_all_checks(bill))


def test_unclear_fee_flagged():
    bill, _, _ = load_sample()
    flagged = [f for f in run_all_checks(bill) if f.rule == "unclear_fee"]
    assert flagged and flagged[0].line_ids == ["L3"]


def test_different_claim_numbers_skip_line_matching():
    bill, eob, _ = load_sample()
    eob.claim_number = "CLM-9999"
    found = rules(run_all_checks(bill, eob))
    assert "claim_number_mismatch" in found
    assert "billed_amount_mismatch" not in found


def test_api_analyze_and_inquiry():
    from fastapi.testclient import TestClient
    import main
    _, _, data = load_sample()
    with TestClient(main.app) as client:
        response = client.post("/bills/analyze", json=data)
    body = response.json()
    assert response.status_code == 200
    assert body["flag_count"] >= 3
    assert "Request for itemized bill" in body["inquiry_draft"]
    assert "Sample Patient" in body["inquiry_draft"]


def test_loader_and_price_compare(tmp_path, monkeypatch):
    monkeypatch.setenv("CLEARCARE_DB", str(tmp_path / "test.db"))
    import importlib
    import database
    importlib.reload(database)
    from care_search import loader
    importlib.reload(loader)

    conn = database.get_connection()
    database.init_db(conn)
    demo = os.path.join(os.path.dirname(os.path.dirname(__file__)), "data", "demo")
    result = loader.load_all(conn, os.path.join(demo, "providers.csv"),
                             os.path.join(demo, "service_prices.csv"))
    assert result["errors"] == []
    assert result["prices_loaded"] == 3

    data = database.compare_prices(conn, "73721", city="Philadelphia")
    assert len(data["results"]) == 3
    joined = " ".join(data["warnings"])
    assert "different price types" in joined
    assert "demonstration data" in joined

    only_cash = database.compare_prices(conn, "73721", price_type="discounted_cash")
    assert [r["price"] for r in only_cash["results"]] == [500.0, 650.0]


def test_loader_reports_bad_rows(tmp_path, monkeypatch):
    monkeypatch.setenv("CLEARCARE_DB", str(tmp_path / "test2.db"))
    import importlib
    import database
    importlib.reload(database)
    from care_search import loader
    importlib.reload(loader)

    providers = tmp_path / "providers.csv"
    prices = tmp_path / "prices.csv"
    providers.write_text(
        "provider_id,name,npi,provider_type,specialty,address,city,state,zip,phone,website,source_url,retrieved_date\n"
        "p1,Real Place,,facility,radiology,,Philadelphia,PA,,,,https://example.com,2026-11-20\n"
    )
    prices.write_text(
        "provider_id,procedure_code,description,price,price_type,payer_name,plan_name,includes,insurance_status,source_url,retrieved_date,notes\n"
        "p1,73721,MRI,abc,discounted_cash,,,,unknown,https://example.com,2026-11-20,\n"
        "p1,73721,MRI,500,made_up_type,,,,unknown,https://example.com,2026-11-20,\n"
        "p1,73721,MRI,500,payer_negotiated,,,,unknown,https://example.com,2026-11-20,\n"
        "p9,73721,MRI,500,discounted_cash,,,,unknown,https://example.com,2026-11-20,\n"
        "p1,73721,MRI,500,discounted_cash,,,,unknown,https://example.com,2026-11-20,ok row\n"
    )
    conn = database.get_connection()
    database.init_db(conn)
    result = loader.load_all(conn, str(providers), str(prices))
    assert result["prices_loaded"] == 1
    assert len(result["errors"]) == 4

import re
from collections import defaultdict

from bill_review.models import Bill, EOB, Finding

# Words that make a charge hard to understand
VAGUE_PATTERNS = [
    r"\bmisc\b", r"\bmiscellaneous\b", r"\bother\b", r"\bservice charge\b",
    r"\bfacility fee\b", r"\bsupplies\b", r"\badjustment\b", r"\bfee\b",
]


def add_up(numbers):
    return round(sum(numbers), 2)


def line_key(date, code, description):
    # Use the code when there is one, otherwise the description
    name = code if code else description.strip().lower()
    return (date, name)


def check_duplicates(bill: Bill):
    findings = []
    groups = defaultdict(list)
    for line in bill.lines:
        key = (
            line.service_date,
            line.code,
            line.description.strip().lower(),
            line.quantity,
            line.charge,
        )
        groups[key].append(line)

    for key, lines in groups.items():
        if len(lines) > 1:
            findings.append(Finding(
                rule="possible_duplicate",
                title="Possible duplicate charge",
                message="These lines have the same date, code, description, quantity, and amount. "
                        "Please confirm whether they are separate services.",
                line_ids=[l.line_id for l in lines],
                evidence={
                    "service_date": key[0],
                    "code": key[1],
                    "description": lines[0].description,
                    "amount_each": key[4],
                    "times_listed": len(lines),
                },
                question=f"Line items {', '.join(l.line_id for l in lines)} look identical "
                         f"({lines[0].description}, ${key[4]:.2f}). Were these separate services?",
            ))
    return findings


def check_subtotal(bill: Bill):
    findings = []
    if bill.stated_subtotal is None:
        return findings
    total = add_up([l.charge for l in bill.lines])
    if abs(total - bill.stated_subtotal) > 0.01:
        findings.append(Finding(
            rule="subtotal_mismatch",
            title="Line items do not match the subtotal",
            message="The listed amounts do not reconcile with the stated subtotal.",
            line_ids=[l.line_id for l in bill.lines],
            evidence={"sum_of_lines": total, "stated_subtotal": bill.stated_subtotal},
            question=f"The line items add up to ${total:.2f} but the subtotal says "
                     f"${bill.stated_subtotal:.2f}. Can you explain the difference?",
        ))
    return findings


def check_unclear_fees(bill: Bill):
    findings = []
    for line in bill.lines:
        text = line.description.strip().lower()
        too_short = len(text) < 4
        vague = any(re.search(p, text) for p in VAGUE_PATTERNS)
        if too_short or vague:
            findings.append(Finding(
                rule="unclear_fee",
                title="Charge is not clearly described",
                message="This description is vague. Ask for itemized detail.",
                line_ids=[line.line_id],
                evidence={"description": line.description, "charge": line.charge},
                question=f"Please explain what \"{line.description}\" (${line.charge:.2f}) covers "
                         f"and provide itemized detail.",
            ))
    return findings


def check_eob_math(eob: EOB):
    findings = []
    for line in eob.lines:
        expected = round(line.billed - line.adjustment - line.plan_paid, 2)
        if abs(expected - line.patient_responsibility) > 0.01:
            findings.append(Finding(
                rule="eob_line_does_not_add_up",
                title="EOB line does not add up",
                message="Billed amount minus adjustment minus plan payment does not equal "
                        "the patient responsibility on this EOB line. There may be a deductible "
                        "or other item not shown. Confirm with your insurer.",
                line_ids=[line.line_id],
                evidence={
                    "billed": line.billed,
                    "adjustment": line.adjustment,
                    "plan_paid": line.plan_paid,
                    "expected_patient_amount": expected,
                    "listed_patient_amount": line.patient_responsibility,
                },
                question="",  # this one is a question for the insurer, not the provider
            ))
    return findings


def check_bill_vs_eob(bill: Bill, eob: EOB):
    findings = []

    # Make sure they are the same claim
    if bill.claim_number and eob.claim_number and bill.claim_number != eob.claim_number:
        findings.append(Finding(
            rule="claim_number_mismatch",
            title="Bill and EOB have different claim numbers",
            message="These documents may not be for the same claim, so line matching was skipped.",
            evidence={"bill_claim": bill.claim_number, "eob_claim": eob.claim_number},
            question="Which claim number does this bill belong to?",
        ))
        return findings

    bill_groups = defaultdict(list)
    for line in bill.lines:
        bill_groups[line_key(line.service_date, line.code, line.description)].append(line)

    eob_groups = defaultdict(list)
    for line in eob.lines:
        eob_groups[line_key(line.service_date, line.code, line.description)].append(line)

    for key, lines in bill_groups.items():
        if not eob.lines:
            continue  # A total-only EOB cannot support service-level comparisons.
        bill_total = add_up([l.charge for l in lines])
        ids = [l.line_id for l in lines]
        if key not in eob_groups:
            findings.append(Finding(
                rule="bill_line_not_on_eob",
                title="Billed service not found on the EOB",
                message="This service is on the bill but not on the EOB. It may be on a different claim.",
                line_ids=ids,
                evidence={"service_date": key[0], "service": key[1], "bill_amount": bill_total},
                question=f"Which claim was {lines[0].description} billed under?",
            ))
            continue
        eob_total = add_up([e.billed for e in eob_groups[key]])
        if abs(bill_total - eob_total) > 0.01:
            findings.append(Finding(
                rule="billed_amount_mismatch",
                title="Bill and EOB show different billed amounts",
                message="The bill total for this service is different from the billed amount on the EOB.",
                line_ids=ids,
                evidence={
                    "service_date": key[0],
                    "service": key[1],
                    "bill_amount": bill_total,
                    "eob_billed_amount": eob_total,
                },
                question=f"The bill lists ${bill_total:.2f} for {lines[0].description} but the "
                         f"insurer's EOB shows ${eob_total:.2f} billed. Why are they different?",
            ))

    # Patient responsibility totals
    eob_total_patient = eob.patient_responsibility_total
    if eob_total_patient is None and eob.lines:
        eob_total_patient = add_up([l.patient_responsibility for l in eob.lines])
    if bill.patient_responsibility is not None and eob_total_patient is not None:
        if abs(bill.patient_responsibility - eob_total_patient) > 0.01:
            findings.append(Finding(
                rule="patient_responsibility_mismatch",
                title="Bill and EOB show different patient amounts",
                message="The bill and EOB show different patient-responsibility amounts.",
                evidence={
                    "bill_patient_responsibility": bill.patient_responsibility,
                    "eob_patient_responsibility": eob_total_patient,
                },
                question=f"My EOB says I owe ${eob_total_patient:.2f} but the bill asks for "
                         f"${bill.patient_responsibility:.2f}. Please explain or correct the balance.",
            ))
    return findings


def run_all_checks(bill: Bill, eob: EOB = None):
    findings = []
    findings += check_duplicates(bill)
    findings += check_subtotal(bill)
    findings += check_unclear_fees(bill)
    if eob is not None:
        findings += check_bill_vs_eob(bill, eob)
        findings += check_eob_math(eob)
    return findings

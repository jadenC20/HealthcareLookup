from datetime import date


def build_inquiry(provider_name, claim_number, findings, patient_name, account_number):
    # Only findings that have a question for the billing office
    questions = [f.question for f in findings if f.question]
    if not questions:
        return None

    lines = []
    lines.append(f"Date: {date.today().isoformat()}")
    lines.append(f"To: Billing Department, {provider_name}")
    lines.append(f"From: {patient_name}")
    lines.append(f"Account number: {account_number}")
    if claim_number:
        lines.append(f"Claim number: {claim_number}")
    lines.append("")
    lines.append("Subject: Request for itemized bill and billing review")
    lines.append("")
    lines.append("Hello,")
    lines.append("")
    lines.append("I am reviewing my recent bill and have a few questions. "
                 "Please send a fully itemized bill with procedure codes, "
                 "and answer the questions below.")
    lines.append("")
    for i, q in enumerate(questions, start=1):
        lines.append(f"{i}. {q}")
    lines.append("")
    lines.append("Please hold this account from collections while the review is open. "
                 "I would also like to know if financial assistance is available.")
    lines.append("")
    lines.append("Thank you,")
    lines.append(patient_name)
    return "\n".join(lines)

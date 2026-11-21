from typing import Optional
from pydantic import BaseModel


class BillLine(BaseModel):
    line_id: str
    service_date: Optional[str] = None
    description: str
    code: Optional[str] = None
    quantity: int = 1
    charge: float
    page: Optional[int] = None  # page in the uploaded document


class Bill(BaseModel):
    provider_name: str
    claim_number: Optional[str] = None
    lines: list[BillLine]
    stated_subtotal: Optional[float] = None
    patient_responsibility: Optional[float] = None


class EOBLine(BaseModel):
    line_id: str
    service_date: Optional[str] = None
    description: str
    code: Optional[str] = None
    billed: float
    plan_paid: float
    adjustment: float
    patient_responsibility: float


class EOB(BaseModel):
    claim_number: Optional[str] = None
    lines: list[EOBLine]
    patient_responsibility_total: Optional[float] = None


class AnalyzeRequest(BaseModel):
    bill: Bill
    eob: Optional[EOB] = None
    patient_name: str = "[Your name]"
    account_number: str = "[Account number]"


class Finding(BaseModel):
    rule: str
    title: str
    message: str
    line_ids: list[str] = []
    evidence: dict = {}
    question: str  # what the patient can ask the billing office

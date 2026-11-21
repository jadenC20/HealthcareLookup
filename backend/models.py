from typing import Optional
from pydantic import BaseModel, Field, ConfigDict

class FiniteModel(BaseModel):
    model_config = ConfigDict(allow_inf_nan=False)


class BillLine(FiniteModel):
    line_id: str
    service_date: Optional[str] = None
    description: str
    code: Optional[str] = None
    quantity: int = Field(default=1,ge=1)
    charge: float
    page: Optional[int] = None  # page in the uploaded document


class Bill(FiniteModel):
    provider_name: str = Field(min_length=1,max_length=200)
    claim_number: Optional[str] = None
    lines: list[BillLine] = Field(max_length=200)
    stated_subtotal: Optional[float] = None
    patient_responsibility: Optional[float] = None


class EOBLine(FiniteModel):
    line_id: str
    service_date: Optional[str] = None
    description: str
    code: Optional[str] = None
    billed: float
    plan_paid: float
    adjustment: float
    patient_responsibility: float


class EOB(FiniteModel):
    claim_number: Optional[str] = None
    lines: list[EOBLine] = Field(max_length=200)
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

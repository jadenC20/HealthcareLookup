import json
import secrets
from datetime import datetime, timezone
from pathlib import Path
from fastapi import APIRouter, HTTPException, Request, Response, UploadFile, File, Form
import database
from account import service as accounts
from bill_review.documents import extract, suggest, MAX_BYTES
from bill_review.reports import report_pdf
from bill_review.models import AnalyzeRequest
from bill_review.checks import run_all_checks
from bill_review.inquiry import build_inquiry

router = APIRouter()

def analyze(data):
    findings=run_all_checks(data.bill,data.eob)
    return {'flag_count':len(findings),'findings':[f.model_dump() for f in findings],
            'inquiry_draft':build_inquiry(data.bill.provider_name,data.bill.claim_number,findings,data.patient_name,data.account_number),
            'note':'Flags are things to confirm, not proof of an error.'}

def pdf_response(data):
    return Response(data,media_type='application/pdf',headers={'Content-Disposition':'attachment; filename="clearcare-report.pdf"','Cache-Control':'no-store'})

@router.post('/documents/extract')
async def extract_document(request:Request,file:UploadFile=File(...),kind:str=Form('bill')):
    accounts.identity(request,True)
    if kind not in ('bill','eob'):
        raise HTTPException(422,'Choose bill or EOB.')
    try:
        data=await file.read(MAX_BYTES+1)
    finally:
        await file.close()
    pages=extract(data,file.filename)
    return {'pages':pages,'suggested':suggest(pages,kind),'kind':kind,
            'warnings':['Review every field against the source before analysis. Layout parsing is best-effort.',
                        'EOB line columns require manual confirmation; blanks are not zero. Original files are not saved.']}

@router.post('/reviews')
def save_review(body:AnalyzeRequest,request:Request):
    user,_=accounts.identity(request,True)
    if not body.bill.lines:
        raise HTTPException(422,'Add at least one reviewed bill line.')
    uid=secrets.token_hex(16)
    payload={'request':body.model_dump(),'analysis':analyze(body)}
    created=datetime.now(timezone.utc).isoformat()
    with database.get_connection() as c:
        c.execute('INSERT INTO reviews VALUES (?,?,?,?,?)',(uid,user['id'],body.bill.provider_name,created,json.dumps(payload)))
    return {'id':uid,'title':body.bill.provider_name,'created':created,'payload':payload}

@router.get('/reviews')
def list_reviews(request:Request):
    user,_=accounts.identity(request)
    with database.get_connection() as c:
        return {'results':[dict(r) for r in c.execute('SELECT id,title,created FROM reviews WHERE user_id=? ORDER BY created DESC',(user['id'],))]}

def owned_review(uid,request,mutate=False):
    user,_=accounts.identity(request,mutate)
    with database.get_connection() as c:
        row=c.execute('SELECT id,title,created,payload FROM reviews WHERE id=? AND user_id=?',(uid,user['id'])).fetchone()
    if not row:
        raise HTTPException(404,'Review not found.')
    result=dict(row)
    result['payload']=json.loads(result['payload'])
    return result

@router.get('/reviews/{uid}')
def get_review(uid:str,request:Request):
    return owned_review(uid,request)

@router.delete('/reviews/{uid}')
def delete_review(uid:str,request:Request):
    owned_review(uid,request,True)
    with database.get_connection() as c:
        c.execute('DELETE FROM reviews WHERE id=?',(uid,))
    return {'ok':True}

@router.get('/reviews/{uid}/pdf')
def saved_pdf(uid:str,request:Request):
    return pdf_response(report_pdf([owned_review(uid,request)]))

@router.post('/reports/pdf')
def current_pdf(body:AnalyzeRequest):
    return pdf_response(report_pdf([{'title':body.bill.provider_name,'payload':{'request':body.model_dump(),'analysis':analyze(body)}}]))


@router.get('/sample')
def sample():
    import json
    return json.loads((Path(__file__).resolve().parent.parent / 'sample/bill_with_problems.json').read_text())


@router.post("/bills/analyze")
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

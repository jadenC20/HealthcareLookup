import hmac
import json
import os
import re
import secrets
import sqlite3
from datetime import datetime, timezone
from fastapi import APIRouter, HTTPException, Request, Response, UploadFile, File, Form
from pydantic import BaseModel, Field
import accounts
import database
from documents import extract, suggest, MAX_BYTES
from reports import report_pdf
from models import AnalyzeRequest
from checks import run_all_checks
from inquiry import build_inquiry

router=APIRouter()

class Credentials(BaseModel):
    email: str = Field(min_length=3,max_length=254)
    password: str = Field(min_length=12,max_length=128)
    name: str = Field(default='',max_length=100)

class Profile(BaseModel):
    name: str = Field(min_length=1,max_length=100)
    city: str = Field(default='',max_length=100)
    plan: str = Field(default='',max_length=200)

def analyze(data):
    findings=run_all_checks(data.bill,data.eob)
    return {'flag_count':len(findings),'findings':[f.model_dump() for f in findings],
            'inquiry_draft':build_inquiry(data.bill.provider_name,data.bill.claim_number,findings,data.patient_name,data.account_number),
            'note':'Flags are things to confirm, not proof of an error.'}

def pdf_response(data):
    return Response(data,media_type='application/pdf',headers={'Content-Disposition':'attachment; filename="clearcare-report.pdf"','Cache-Control':'no-store'})

@router.post('/auth/register')
def register(body:Credentials,request:Request,response:Response):
    accounts.throttle(request)
    email=body.email.strip().lower()
    if not re.fullmatch(r'[^\s@]+@[^\s@]+\.[^\s@]+',email) or not body.name.strip():
        raise HTTPException(422,'Enter a valid email and display name.')
    uid=secrets.token_hex(16)
    try:
        with database.get_connection() as c:
            c.execute('INSERT INTO users(id,email,name,password) VALUES (?,?,?,?)',(uid,email,body.name.strip(),accounts.password_hash(body.password)))
    except sqlite3.IntegrityError:
        raise HTTPException(409,'Unable to register with this email. Try signing in.')
    csrf=accounts.issue(response,uid,os.getenv('CLEARCARE_SECURE_COOKIES')=='1')
    user,_=lookup(uid)
    return {'user':accounts.public(user),'csrf':csrf}

def lookup(uid):
    with database.get_connection() as c:
        return c.execute('SELECT * FROM users WHERE id=?',(uid,)).fetchone(),None

@router.post('/auth/login')
def login(body:Credentials,request:Request,response:Response):
    accounts.throttle(request)
    with database.get_connection() as c:
        user=c.execute('SELECT * FROM users WHERE email=?',(body.email.strip().lower(),)).fetchone()
    stored=user['password'] if user else accounts.password_hash('dummy-not-a-password')
    if not hmac.compare_digest(stored,accounts.password_hash(body.password,stored.split(':')[0])) or not user:
        raise HTTPException(401,'Email or password is incorrect.')
    # Rotate an existing browser session on login.
    old=request.cookies.get(accounts.COOKIE)
    if old:
        import hashlib
        with database.get_connection() as c:
            c.execute('DELETE FROM sessions WHERE token=?',(hashlib.sha256(old.encode()).hexdigest(),))
    return {'user':accounts.public(user),'csrf':accounts.issue(response,user['id'],os.getenv('CLEARCARE_SECURE_COOKIES')=='1')}

@router.get('/auth/me')
def me(request:Request):
    user,session=accounts.identity(request)
    return {'user':accounts.public(user),'csrf':session['csrf']}

@router.post('/auth/logout')
def logout(request:Request,response:Response):
    _,session=accounts.identity(request,True)
    with database.get_connection() as c:
        c.execute('DELETE FROM sessions WHERE token=?',(session['token'],))
    response.delete_cookie(accounts.COOKIE,path='/')
    return {'ok':True}

@router.patch('/profile')
def profile(body:Profile,request:Request):
    user,_=accounts.identity(request,True)
    with database.get_connection() as c:
        c.execute('UPDATE users SET name=?,city=?,plan=? WHERE id=?',(body.name,body.city,body.plan,user['id']))
    return {'user':accounts.public(lookup(user['id'])[0])}

@router.delete('/account')
def delete_account(request:Request,response:Response):
    user,_=accounts.identity(request,True)
    with database.get_connection() as c:
        c.execute('DELETE FROM reviews WHERE user_id=?',(user['id'],))
        c.execute('DELETE FROM sessions WHERE user_id=?',(user['id'],))
        c.execute('DELETE FROM users WHERE id=?',(user['id'],))
    response.delete_cookie(accounts.COOKIE,path='/')
    return {'ok':True}

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

@router.get('/account/pdf')
def account_pdf(request:Request):
    user,_=accounts.identity(request)
    with database.get_connection() as c:
        rows=[dict(r) for r in c.execute('SELECT * FROM reviews WHERE user_id=? ORDER BY created',(user['id'],))]
    for row in rows:
        row['payload']=json.loads(row['payload'])
    return pdf_response(report_pdf(rows,accounts.public(user)))

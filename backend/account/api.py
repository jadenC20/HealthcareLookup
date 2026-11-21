import hmac
import os
import re
import secrets
import sqlite3
from fastapi import APIRouter, HTTPException, Request, Response
from pydantic import BaseModel, Field
import database
from account import service as accounts
from bill_review.reports import report_pdf
import json

router = APIRouter()

class Credentials(BaseModel):
    email: str = Field(min_length=3,max_length=254)
    password: str = Field(min_length=12,max_length=128)
    name: str = Field(default='',max_length=100)

class Profile(BaseModel):
    name: str = Field(min_length=1,max_length=100)
    city: str = Field(default='',max_length=100)
    plan: str = Field(default='',max_length=200)

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

@router.get('/account/pdf')
def account_pdf(request:Request):
    user,_=accounts.identity(request)
    with database.get_connection() as c:
        rows=[dict(r) for r in c.execute('SELECT * FROM reviews WHERE user_id=? ORDER BY created',(user['id'],))]
    for row in rows:
        row['payload']=json.loads(row['payload'])
    return pdf_response(report_pdf(rows,accounts.public(user)))

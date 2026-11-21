"""Local account storage. Sessions never expose password hashes to clients."""
import hashlib
import hmac
import secrets
import time
from fastapi import HTTPException, Request
import database

COOKIE = 'clearcare_session'
SESSION_SECONDS = 60 * 60 * 12

def init():
    with database.get_connection() as c:
        c.executescript('''
        CREATE TABLE IF NOT EXISTS users (
          id TEXT PRIMARY KEY, email TEXT UNIQUE NOT NULL, name TEXT NOT NULL,
          password TEXT NOT NULL, city TEXT DEFAULT '', plan TEXT DEFAULT '');
        CREATE TABLE IF NOT EXISTS sessions (
          token TEXT PRIMARY KEY, user_id TEXT NOT NULL, csrf TEXT NOT NULL, expires REAL NOT NULL);
        CREATE TABLE IF NOT EXISTS reviews (
          id TEXT PRIMARY KEY, user_id TEXT NOT NULL, title TEXT NOT NULL,
          created TEXT NOT NULL, payload TEXT NOT NULL);
        CREATE TABLE IF NOT EXISTS login_attempts (
          identity TEXT PRIMARY KEY, count INTEGER NOT NULL, started REAL NOT NULL);
        ''')

def password_hash(password, salt=None):
    salt = salt or secrets.token_hex(16)
    digest = hashlib.scrypt(password.encode(), salt=bytes.fromhex(salt), n=16384, r=8, p=1).hex()
    return salt + ':' + digest

def public(user):
    return {key: user[key] for key in ('id', 'email', 'name', 'city', 'plan')}

def identity(request: Request, mutate=False):
    raw = request.cookies.get(COOKIE, '')
    token = hashlib.sha256(raw.encode()).hexdigest()
    with database.get_connection() as c:
        session = c.execute('SELECT * FROM sessions WHERE token=? AND expires>?', (token,time.time())).fetchone()
        if not session:
            raise HTTPException(401, 'Please sign in first.')
        if mutate and not hmac.compare_digest(request.headers.get('x-csrf-token', ''), session['csrf']):
            raise HTTPException(403, 'Session verification failed. Refresh and try again.')
        user = c.execute('SELECT * FROM users WHERE id=?', (session['user_id'],)).fetchone()
    if not user:
        raise HTTPException(401, 'Please sign in first.')
    return user, session

def issue(response, user_id, secure=False):
    token, csrf = secrets.token_urlsafe(32), secrets.token_urlsafe(32)
    with database.get_connection() as c:
        c.execute('DELETE FROM sessions WHERE expires<?', (time.time(),))
        c.execute('INSERT INTO sessions VALUES (?,?,?,?)',
                  (hashlib.sha256(token.encode()).hexdigest(),user_id,csrf,time.time()+SESSION_SECONDS))
    response.set_cookie(COOKIE,token,httponly=True,samesite='strict',secure=secure,max_age=SESSION_SECONDS,path='/')
    return csrf

def throttle(request):
    key = request.client.host if request.client else 'local'
    with database.get_connection() as c:
        row = c.execute('SELECT * FROM login_attempts WHERE identity=?',(key,)).fetchone()
        if row and row['started'] > time.time()-900 and row['count'] >= 25:
            raise HTTPException(429,'Too many attempts. Try again in 15 minutes.')
        if not row or row['started'] < time.time()-900:
            c.execute('INSERT OR REPLACE INTO login_attempts VALUES (?,1,?)',(key,time.time()))
        else:
            c.execute('UPDATE login_attempts SET count=count+1 WHERE identity=?',(key,))

from contextlib import asynccontextmanager
from fastapi import FastAPI, Request
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse
from account import service as accounts
from account.api import router as account_router
from bill_review.api import router as bill_router
from care_search.api import router as care_router, reload_data

@asynccontextmanager
async def lifespan(app):
    reload_data()
    accounts.init()
    yield


app = FastAPI(title="ClearCare API", lifespan=lifespan)
app.include_router(account_router)
app.include_router(bill_router)
app.include_router(care_router)
@app.middleware('http')
async def protect_requests(request: Request, call_next):
    allowed = {'http://localhost:4173','http://127.0.0.1:4173','http://localhost:8000','http://127.0.0.1:8000'}
    if request.method not in ('GET','HEAD','OPTIONS') and request.headers.get('origin') and request.headers['origin'] not in allowed:
        return JSONResponse({'detail':'Origin not allowed.'},status_code=403)
    try:
        size=int(request.headers.get('content-length','0'))
    except ValueError:
        return JSONResponse({'detail':'Invalid request size.'},status_code=400)
    if size > 11*1024*1024:
        return JSONResponse({'detail':'Request exceeds 11 MB.'},status_code=413)
    response=await call_next(request)
    response.headers['Cache-Control']='no-store'
    response.headers['X-Content-Type-Options']='nosniff'
    return response

app.add_middleware(
    CORSMiddleware,
    allow_origins=["http://localhost:4173", "http://127.0.0.1:4173"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)


@app.get("/health")
def health():
    return {"status": "ok"}

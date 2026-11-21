import io
import json
import sys
from pathlib import Path
import pytest
import fitz
from PIL import Image, ImageDraw, ImageFont
from fastapi.testclient import TestClient

sys.path.insert(0,str(Path(__file__).resolve().parents[1]))
import main
import database
from documents import extract

@pytest.fixture
def client(tmp_path,monkeypatch):
    monkeypatch.setattr(database,'DB_PATH',str(tmp_path/'accounts.db'))
    with TestClient(main.app) as c:
        yield c

def register(client,email='test@example.org'):
    response=client.post('/auth/register',json={'email':email,'password':'test-password-12345','name':'Test User'})
    assert response.status_code==200,response.text
    return {'x-csrf-token':response.json()['csrf']}

def sample():
    return json.loads((Path(__file__).resolve().parents[1]/'sample/bill_with_problems.json').read_text())

def pdf_bytes():
    doc=fitz.open()
    page=doc.new_page()
    page.insert_text((45,60),'Provider: Test Clinic\nClaim number: TEST-1\nOffice visit 120.00\nSubtotal: 120.00\nPatient responsibility: 25.00',fontsize=14)
    return doc.tobytes()

def test_account_save_export_isolation_and_deletion(client):
    headers=register(client)
    assert client.patch('/profile',headers=headers,json={'name':'Updated','city':'Dover','plan':'Example plan'}).status_code==200
    assert client.post('/reviews',json=sample()).status_code==403
    review=client.post('/reviews',headers=headers,json=sample()).json()
    uid=review['id']
    assert len(client.get('/reviews').json()['results'])==1
    pdf=client.get(f'/reviews/{uid}/pdf')
    assert pdf.content.startswith(b'%PDF-')
    doc=fitz.open(stream=pdf.content,filetype='pdf')
    text=''.join(p.get_text() for p in doc)
    assert 'Sample Patient' in text and 'Possible duplicate' in text
    assert client.get('/account/pdf').content.startswith(b'%PDF-')
    assert client.post('/auth/logout',headers=headers).status_code==200
    assert client.get('/reviews').status_code==401
    other=register(client,'other@example.org')
    for path in (f'/reviews/{uid}',f'/reviews/{uid}/pdf'):
        assert client.get(path).status_code==404
    assert client.delete(f'/reviews/{uid}',headers=other).status_code==404
    assert client.delete('/account',headers=other).status_code==200
    login=client.post('/auth/login',json={'email':'test@example.org','password':'test-password-12345'}).json()
    headers={'x-csrf-token':login['csrf']}
    assert client.get('/auth/me').json()['user']['city']=='Dover'
    assert client.delete(f'/reviews/{uid}',headers=headers).status_code==200
    assert client.get('/reviews').json()['results']==[]

def test_upload_validation_and_text_pdf(client):
    assert client.post('/documents/extract',files={'file':('x.pdf',pdf_bytes(),'application/pdf')}).status_code==401
    headers=register(client)
    result=client.post('/documents/extract',headers=headers,files={'file':('x.pdf',pdf_bytes(),'application/pdf')})
    assert result.status_code==200,result.text
    data=result.json()
    assert data['suggested']['provider_name']=='Test Clinic'
    assert data['suggested']['lines'][0]['charge']==120
    assert data['suggested']['patient_responsibility']==25
    assert data['pages'][0]['page']==1
    assert client.post('/documents/extract',headers=headers,files={'file':('x.pdf',b'not a PDF')}).status_code==415
    assert client.post('/documents/extract',headers=headers,files={'file':('x.html',b'hello')}).status_code==415
    assert client.post('/documents/extract',headers=headers,files={'file':('x.pdf',b'x'*(10*1024*1024+1))}).status_code==413
    locked=fitz.open(stream=pdf_bytes(),filetype='pdf').tobytes(encryption=fitz.PDF_ENCRYPT_AES_256,owner_pw='secret',user_pw='secret')
    assert client.post('/documents/extract',headers=headers,files={'file':('x.pdf',locked)}).status_code==422

def test_origin_csrf_and_password(client):
    assert client.post('/auth/register',headers={'origin':'https://evil.example'},json={}).status_code==403
    assert client.post('/auth/register',json={'name':'X','email':'x@example.org','password':'short'}).status_code==422
    register(client)
    assert client.post('/auth/login',json={'email':'test@example.org','password':'wrong-password-123'}).status_code==401
    assert client.delete('/account',headers={'x-csrf-token':'wrong'}).status_code==403

def test_missing_eob_totals_are_not_assumed_zero(client):
    data=sample()
    data['eob']={'lines':[]}
    response=client.post('/bills/analyze',json=data).json()
    assert 'patient_responsibility_mismatch' not in [r['rule'] for r in response['findings']]

@pytest.mark.parametrize('extension,format',[('.png','PNG'),('.jpg','JPEG')])
def test_local_image_ocr(extension,format):
    image=Image.new('RGB',(1400,400),'white')
    font=ImageFont.truetype('C:/Windows/Fonts/arial.ttf',32) if Path('C:/Windows/Fonts/arial.ttf').exists() else ImageFont.load_default(size=32)
    ImageDraw.Draw(image).multiline_text((35,40),'Provider: Test Clinic\nOffice visit 120.00\nSubtotal: 120.00',font=font,fill='black',spacing=18)
    out=io.BytesIO();image.save(out,format=format)
    pages=extract(out.getvalue(),'bill'+extension)
    assert '120.00' in pages[0]['text']

def test_scanned_pdf_ocr():
    image=Image.new('RGB',(1400,400),'white')
    font=ImageFont.truetype('C:/Windows/Fonts/arial.ttf',40) if Path('C:/Windows/Fonts/arial.ttf').exists() else ImageFont.load_default(size=40)
    ImageDraw.Draw(image).text((30,50),'Office visit 120.00',font=font,fill='black')
    out=io.BytesIO();image.save(out,format='PNG')
    doc=fitz.open();page=doc.new_page(width=700,height=200);page.insert_image(page.rect,stream=out.getvalue())
    pages=extract(doc.tobytes(),'scan.pdf')
    assert pages[0]['method']=='ocr' and '120.00' in pages[0]['text']

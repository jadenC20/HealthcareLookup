"""Bounded, local-only PDF and image extraction; no documents are retained."""
import io
import os
import re
import subprocess
import shutil
from pathlib import Path
from fastapi import HTTPException
from PIL import Image, ImageOps, UnidentifiedImageError
import fitz
import pytesseract

MAX_BYTES = 10 * 1024 * 1024
MAX_PAGES = 12
Image.MAX_IMAGE_PIXELS = 20_000_000
if os.environ.get('TESSERACT_CMD'):
    pytesseract.pytesseract.tesseract_cmd = os.environ['TESSERACT_CMD']

def ocr(image):
    if image.width * image.height > 20_000_000:
        raise HTTPException(413,'Image exceeds the 20 megapixel limit.')
    try:
        return pytesseract.image_to_string(ImageOps.exif_transpose(image).convert('RGB'), timeout=25)
    except pytesseract.TesseractNotFoundError:
        here=Path(__file__).resolve().parent
        if not (here/'ocr-data/eng.traineddata.gz').exists():
            raise HTTPException(503,'Run node backend/setup-ocr.mjs once to install the local OCR model.')
        data=io.BytesIO()
        ImageOps.exif_transpose(image).convert('RGB').save(data,format='PNG')
        try:
            result=subprocess.run([shutil.which('node') or 'node',str(here/'ocr.mjs')],input=data.getvalue(),capture_output=True,timeout=45)
            if result.returncode:
                raise HTTPException(503,'Local OCR engine failed. Run npm install and check the OCR model setup.')
            return result.stdout.decode('utf-8')
        except (subprocess.TimeoutExpired,FileNotFoundError):
            raise HTTPException(503,'Local OCR is unavailable or timed out. Try a smaller image.')
    except RuntimeError:
        raise HTTPException(422,'OCR timed out. Try a clearer image or a shorter PDF.')

def extract(data, filename):
    suffix = os.path.splitext(filename or '')[1].lower()
    if suffix not in ('.pdf','.png','.jpg','.jpeg'):
        raise HTTPException(415,'Supported files: PDF, PNG, and JPEG.')
    if len(data)>MAX_BYTES:
        raise HTTPException(413,'Each file must be at most 10 MB.')
    pages=[]
    try:
        if suffix=='.pdf':
            if not data.startswith(b'%PDF-'):
                raise HTTPException(415,'File contents do not match a PDF.')
            with fitz.open(stream=data,filetype='pdf') as doc:
                if doc.needs_pass:
                    raise HTTPException(422,'Password-protected PDFs must be unlocked before upload.')
                if not 0 < len(doc) <= MAX_PAGES:
                    raise HTTPException(413,f'Use a PDF with 1–{MAX_PAGES} pages.')
                for number,page in enumerate(doc,1):
                    text=page.get_text(sort=True).strip()
                    method='text'
                    if len(text)<30:
                        if page.rect.width*page.rect.height*4>20_000_000:
                            raise HTTPException(413,'PDF page dimensions exceed the OCR limit.')
                        pix=page.get_pixmap(matrix=fitz.Matrix(2,2))
                        text=ocr(Image.open(io.BytesIO(pix.tobytes('png'))))
                        method='ocr'
                    pages.append({'page':number,'text':text[:40000],'method':method})
        else:
            with Image.open(io.BytesIO(data)) as image:
                expected='PNG' if suffix=='.png' else 'JPEG'
                if image.format!=expected:
                    raise HTTPException(415,'Image contents do not match its extension.')
                pages=[{'page':1,'text':ocr(image)[:40000],'method':'ocr'}]
    except HTTPException:
        raise
    except (UnidentifiedImageError, Image.DecompressionBombError, Image.DecompressionBombWarning, ValueError, RuntimeError):
        raise HTTPException(422,'Unable to read this file. Use a valid, readable PDF or image.')
    if not any(p['text'].strip() for p in pages):
        raise HTTPException(422,'No readable text found. Try a clearer scan.')
    return pages

def suggest(pages, kind):
    """Conservative line-ending amount parser. User must confirm every field."""
    lines=[]
    text='\n'.join(p['text'] for p in pages)
    def labeled(label):
        match=re.search(label+r'\s*[:#]?\s*\$?([\d,]+\.\d{2})',text,re.I)
        return float(match[1].replace(',','')) if match else None
    claim=re.search(r'Claim\s*(?:number|no\.?|#)\s*:?\s*([\w-]+)',text,re.I)
    provider=re.search(r'Provider(?: name)?\s*:\s*(.+)',text,re.I)
    for p in pages:
        for raw in p['text'].splitlines():
            m=re.match(r'(.+?)\s+\$?([\d,]+\.\d{2})\s*$',raw.strip())
            if not m or re.search(r'total|balance|responsibility|payment|adjustment|paid|deductible|coinsurance',m[1],re.I):
                continue
            description=m[1].strip()
            code=re.search(r'\b(\d{5}|[A-Z]\d{4})\b',description)
            date=re.search(r'\b(\d{4}-\d{2}-\d{2}|\d{1,2}/\d{1,2}/\d{4})\b',description)
            lines.append({'line_id':f'L{len(lines)+1}','description':description,'charge':float(m[2].replace(',','')),
                          'code':code[1] if code else None,'service_date':date[1] if date else None,'page':p['page'],'quantity':1})
    if kind=='eob':
        return {'claim_number':claim[1] if claim else None,'patient_responsibility_total':labeled(r'Patient responsibility'), 'lines':[]}
    return {'provider_name':provider[1].strip() if provider else '', 'claim_number':claim[1] if claim else None,
            'lines':lines[:200], 'stated_subtotal':labeled(r'Subtotal'), 'patient_responsibility':labeled(r'(?:Patient responsibility|Balance due|Amount due)')}

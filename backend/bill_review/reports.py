import io
import json
from html import escape
from reportlab.lib import colors
from reportlab.lib.styles import getSampleStyleSheet
from reportlab.platypus import SimpleDocTemplate, Paragraph, Spacer, PageBreak, KeepTogether

def report_pdf(records, profile=None):
    out=io.BytesIO()
    styles=getSampleStyleSheet()
    styles['BodyText'].leading=14
    flow=[]
    def text(value, style='BodyText'):
        # Built-in PDF fonts cover western text; replace unsupported glyphs explicitly.
        value=str(value).encode('cp1252',errors='replace').decode('cp1252')
        flow.extend([Paragraph(escape(value).replace('\n','<br/>'),styles[style]),Spacer(1,6)])
    text('ClearCare | Healthcare cost review','Title')
    text('Review flags are questions to confirm, not proof of errors or a final amount owed.')
    if profile:
        text('Profile','Heading2')
        for key in ('name','email','city','plan'):
            text(f'{key.title()}: {profile.get(key, "")}')
    for record in records:
        text(record.get('title','Bill review'),'Heading1')
        text(record.get('created',''))
        payload=record['payload']
        bill=payload['request']['bill']
        text('Provider: '+bill['provider_name'])
        text('Claim: '+str(bill.get('claim_number') or 'Not supplied'))
        text('Reviewed line items','Heading2')
        for line in bill['lines']:
            text(f"{line['line_id']} | Page {line.get('page') or '?'} | {line['description']} | ${line['charge']:.2f}")
        eob=payload['request'].get('eob')
        if eob:
            text('EOB supplied','Heading2')
            text(f"Claim: {eob.get('claim_number') or 'Not supplied'} | Patient responsibility: {eob.get('patient_responsibility_total')}")
            for line in eob['lines']:
                text(f"{line['line_id']}: {line['description']} | Billed ${line['billed']:.2f}, paid ${line['plan_paid']:.2f}, adjustment ${line['adjustment']:.2f}, patient ${line['patient_responsibility']:.2f}")
        text('Findings','Heading2')
        if not payload['analysis']['findings']:
            text('No flags from the implemented checks. This does not establish that the bill is correct.')
        for finding in payload['analysis']['findings']:
            start=len(flow)
            text(finding['title'],'Heading3')
            text(finding['message'])
            text('Evidence: '+json.dumps(finding['evidence'],ensure_ascii=True))
            text('Lines: '+', '.join(finding.get('line_ids',[])))
            block=flow[start:]
            del flow[start:]
            flow.append(KeepTogether(block))
        flow.append(PageBreak())
        text('Billing inquiry draft','Heading2')
        text(payload['analysis']['inquiry_draft'])
    def footer(canvas,doc):
        canvas.setFillColor(colors.HexColor('#496b60'))
        canvas.setFont('Helvetica',9)
        canvas.drawString(42,25,'ClearCare - Private export - Review before sharing')
        canvas.drawRightString(550,25,str(doc.page))
    SimpleDocTemplate(out,pagesize=(595,842),leftMargin=42,rightMargin=42,topMargin=42,bottomMargin=45).build(flow,onFirstPage=footer,onLaterPages=footer)
    return out.getvalue()

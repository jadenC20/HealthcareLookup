const details={insurer:'Example Insurance',plan_name:'Example Silver Variant',plan_id:'PUBLIC-PLAN',network:'Example Network',state:'PA',year:2026,employer:'Example Employer'};
const candidate={id:'candidate-1',details,url:'https://example.org/benefits.pdf',source_url:'https://example.org/plans',source:'web_search',match:'needs_confirmation'};
const result={status:'partial',may_be_inaccurate:true,caution:'Verify these partial results.',details,details_confirmed:true,candidates:[candidate],selected:candidate,document_details:{...details,plan_name:'Document Silver Variant'},document:{content_type:'application/pdf',source_url:candidate.url,sha256:'synthetic-fingerprint',retrieved_at:'2026-11-21T10:00:00Z'},benefits:[{name:'Individual deductible',value:'$2,000 in network; exceptions apply',quote:'The deductible is $2,000.',page:3}],steps:[{name:'interpret_sbc',status:'complete'}],cautions:[{step:'interpret_sbc',code:'missing_benefits',message:'Some benefits were not found.',missing:['emergency_room'],action:'Confirm with your insurer.'}],updated:'2026-11-21T10:01:00Z'};


export {details,candidate,result};

import test from 'node:test';
import assert from 'node:assert/strict';
import {insuranceView,insuranceResults,sourceLink,billAdviceHTML,careAdviceHTML} from '../insurance-view.mjs';

import {result,candidate} from './fixtures/insurance.mjs';

test('renders all public insurance result fields, source evidence, warnings and recovery controls',()=>{
 const html=insuranceView({revision:'revision-1',result},{has_card:true,updated:'2026-11-21T09:00:00Z',lookup_status:'partial'});
 for(const text of ['Example Insurance','PUBLIC-PLAN','Example Network','Example Employer','Document Silver Variant','Verify these partial results.','You confirmed','Selected benefits document','needs confirmation','web search','https://example.org/plans','synthetic-fingerprint','application/pdf','Some benefits were not found.','emergency room','Confirm with your insurer.','missing_benefits','interpret sbc','complete','$2,000 in network','The deductible is $2,000.','PDF page 3','benefits.pdf#page=3','Download saved card','Download saved SBC','Print / save analysis PDF','Retry automated lookup','data-candidate="0"','benefit-search'])assert.ok(html.includes(text),`Missing ${text}`);
 assert.ok(!html.includes('revision-1'),'Revision is not an AI insight');
});
test('unstarted and null collections do not fabricate benefits, confirmation or missing plan values',()=>{
 const html=insuranceResults({status:'not_started',details:{},candidates:null,benefits:null,cautions:null,steps:null});
 assert.match(html,/No interpreted benefits/);assert.match(html,/have not been confirmed/);assert.match(html,/Not found/);assert.doesNotMatch(html,/<dd>0<|undefined|null/);
});
test('AI and source text stays inert and unsupported source URLs are never linked',()=>{
 const html=insuranceResults({...result,benefits:[{name:'<img src=x>',value:'<script>bad()</script>',quote:'" onmouseover="bad()',page:2}],selected:{...candidate,url:'javascript:alert(1)'},document:{source_url:'javascript:alert(1)'},cautions:[{message:'<svg onload=bad()>',missing:['<script>'],action:'<iframe>'}]});
 assert.doesNotMatch(html,/<script>|<img|<svg|<iframe|href="javascript:/);assert.match(html,/&lt;script&gt;/);assert.equal(sourceLink('data:text/html,bad'),'');assert.equal(sourceLink('https://secret@example.org/file'),'');
});
test('AI bill and care displays distinguish failures and quote only returned evidence',()=>{
 assert.match(billAdviceHTML({status:'unavailable',detail:'Key rejected'}),/Key rejected/);assert.equal(billAdviceHTML(undefined),'');
 const bill=billAdviceHTML({status:'completed',result:{summary:'<script>bad</script>',items:[{line_ids:['L1'],explanation:'Clarify the service',question:'Itemize?'}],limitations:['Not proof of overcharging']}});assert.doesNotMatch(bill,/<script>/);assert.match(bill,/L1/);
 const care=careAdviceHTML({results:[{name:'Real listing'}],insurance_context:{plan_label:'Saved plan',benefits:[{name:'Visit',quote:'Exact plan quote',page:3,value:'See plan'}]},ai:{status:'completed',result:{summary:'Ask about restrictions',recommendations:[{listing_index:0,explanation:'Confirm referral',questions:[],benefit_indices:[0]}],limitations:[]}}});
 for(const text of ['Real listing','Saved plan','Exact plan quote','Page 3','unverified'])assert.ok(care.includes(text));
});

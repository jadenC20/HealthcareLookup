import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';
import {validateUpload,priceView} from '../api-client.mjs';
import {insuranceView,careAdviceHTML} from '../insurance-view.mjs';

const source=(await readFile(new URL('../connected.mjs',import.meta.url),'utf8')).replace(/^import .*;\n/gm,'').replace(/\ninitialize\(\);\s*$/,'');
function node(){return {isConnected:true,innerHTML:'',textContent:'',disabled:false,childNodes:[],dataset:{},classList:{add(){},remove(){},toggle(){}},append(){},prepend(){},before(){},remove(){},setAttribute(){},removeAttribute(){},insertAdjacentHTML(){},addEventListener(){},querySelectorAll(){return [];},querySelector(){return null;}};}
function harness(request=async()=>({}),elements=new Map()){
 const timers=[],dialog={...node(),open:false,close(){this.open=false;this.closed=true;}},main=node();
 const context=vm.createContext({console,URL,URLSearchParams,FormData,validateUpload,priceView,insuranceView,careAdviceHTML,Promise,Date,Number,Map,Set,Object,String,JSON,Error,document:{querySelector:selector=>elements.get(selector)||null,querySelectorAll:()=>[],createElement:()=>node()},window:{addEventListener(){}},dialog,main,activePage:'',escaped:String,notify(){},route(){},render(){},titles:{},showProfile(){},showBenefits(){},setTimeout(fn,ms){timers.push({fn,ms});return timers.length;},clearTimeout(){},setInterval(){return 1;},clearInterval(){},createSupabaseAuth:()=>({config:{configured:true},getAccessToken:async()=>null}),createApi:()=>({request,clearSession(){context.sessionCleared=true;},setSession(){}})});
 vm.runInContext(source,context);
 const run=code=>vm.runInContext(code,context);
 run("user={id:'owner'};cloudUser={id:'owner'};");
 return {context,run,dialog,elements,timers};
}
function billHarness(request){
 const host=node(),form=node(),upload=node(),file=node(),reviewButton={...node(),value:'ai'},label=node(),consent={...node(),lastChild:{textContent:''}},checkbox={checked:true},nodes=new Map();
 form.elements=Object.fromEntries(['provider_name','claim_number','stated_subtotal','patient_responsibility','patient_name','account_number','eob','ai_consent'].map(key=>[key,{...node(),value:'',checked:true,closest:()=>node()}]));
 file.files=[Object.assign(new Blob(['bill'],{type:'application/pdf'}),{name:'new-bill.pdf'})];file.closest=()=>label;upload.elements={file};
 consent.querySelector=()=>checkbox;upload.querySelector=selector=>selector==='.confirm'?consent:node();
 form.querySelector=selector=>selector==='[value="ai"]'?reviewButton:node();form.querySelectorAll=()=>[];
 for(const [selector,value] of Object.entries({'#manual-bill':form,'#ai-bill-upload':upload,'#medical-bill-file':file,'#audit-review':reviewButton}))nodes.set(selector,value);
 host.querySelector=selector=>{if(!nodes.has(selector))nodes.set(selector,node());return nodes.get(selector);};
 const root=node(),h=harness(request,new Map([['#live-page-content',root]]));
 h.context.document.createElement=tag=>tag==='section'?host:node();
 h.run("review={bill:{provider_name:'Previous bill',lines:[]}};billEditor();");
 return {...h,host,form,upload,file,reviewButton,nodes};
}

test('insurance wizard refreshes processing results from Bill/Care without an account panel',async()=>{
 const calls=[],modal={...node(),id:'insurance-wizard'};
 const state={revision:'r1',result:{status:'processing'}};
 const h=harness(async path=>{calls.push(path);return path.endsWith('insurance-card')?{has_card:true}:state;},new Map([['#insurance-wizard',modal]]));
 h.dialog.open=true;h.run('renderInsurance=(target,state)=>{globalThis.renderedState=state;};');
 await h.run('loadInsurance()');
 assert.equal(calls.length,2);assert.equal(h.context.renderedState,state);assert.ok(h.timers.some(timer=>timer.ms===4000));
});

test('sign-out closes private modals and clears print previews and in-memory API session',()=>{
 const print=node(),h=harness(undefined,new Map([['.print-letter',print]]));let removed=false;print.remove=()=>{removed=true;};h.dialog.open=true;
 h.run("review={private:true};analysis={private:true};insuranceState={private:true};handleCloudIdentity(null);");
 assert.equal(h.dialog.closed,true);assert.equal(removed,true);assert.equal(h.context.sessionCleared,true);
 assert.equal(h.run('review'),null);assert.equal(h.run('analysis'),null);assert.equal(h.run('insuranceState'),null);
});

test('failed bill extraction remains retryable when a prior review exists',async()=>{
 let extracts=0;const h=billHarness(async path=>{if(path==='/documents/extract'){extracts++;throw new Error('Temporary model error');}return {};});
 let submitted=0;h.form.requestSubmit=()=>{submitted++;};
 const event={preventDefault(){},target:h.upload,submitter:h.reviewButton};
 await h.upload.onsubmit(event);assert.equal(h.reviewButton.disabled,false);
 h.reviewButton.onclick({preventDefault(){}});await new Promise(setImmediate);assert.equal(extracts,2);assert.equal(submitted,0);
 assert.equal(h.nodes.get('#bill-api-status').textContent,'Temporary model error');
 assert.equal(h.run('review.bill.provider_name'),'Previous bill');
});

test('a bill extraction finishing after sign-out cannot restore private bill state',async()=>{
 let resolveExtract;const h=billHarness(path=>path==='/documents/extract'?new Promise(resolve=>{resolveExtract=resolve;}):Promise.resolve({}));
 const pending=h.upload.onsubmit({preventDefault(){},target:h.upload,submitter:h.reviewButton});
 h.run('user=null;cloudUser=null;review=null;analysis=null;');
 resolveExtract({extraction:{bill:{provider_name:'Private bill',lines:[]}}});await pending;
 assert.equal(h.run('review'),null);assert.equal(h.run('analysis'),null);assert.equal(h.reviewButton.disabled,false);
});

test('plan confirmation preserves an SBC coverage year and leaves an unknown year blank',()=>{
 for(const [documentDetails,expected] of [[{year:2025},2025],[undefined,'']]){
  const h=harness(),target={...node(),id:'insurance-wizard'},form=node(),nodes=new Map();form.elements=Object.fromEntries(['insurer','state','year'].map(key=>[key,{value:''}]));form.closest=()=>node();
  target.querySelector=selector=>{if(selector==='#insurance-details')return form;if(!nodes.has(selector))nodes.set(selector,node());return nodes.get(selector);};
  h.context.target=target;h.context.documentDetails=documentDetails;
  h.run("renderInsurance(target,{revision:'r1',result:{details_confirmed:false,details:{},document_details:documentDetails}},{})");
  assert.equal(form.elements.year.value,expected);
 }
});

test('profile care tools search plain-language services for prices, AI guidance, and providers',async()=>{
 for(const [mode,useAI,endpoint] of [['prices',false,'/prices/compare'],['prices',true,'/care/recommendations'],['providers',false,'/providers']]){
  const paths=[],host=node(),form={...node(),values:{q:' MRI knee ',city:'Philadelphia',specialty:'radiology',price_type:'discounted_cash',...(useAI?{use_plan_ai:'on',ai_consent:'on'}:{})}},nodes=new Map([['form',form]]);
  host.querySelector=selector=>{if(!nodes.has(selector))nodes.set(selector,node());return nodes.get(selector);};
  const h=harness(async path=>{paths.push(path);return {results:[]};});h.context.document.createElement=()=>host;h.context.container=node();h.context.FormData=class{constructor(form){return Object.entries(form.values)[Symbol.iterator]();}};
  h.run('careProfileSettings(container)');await form.onsubmit({preventDefault(){},target:form,submitter:{value:mode}});
  assert.equal(paths.length,1);const query=new URL(paths[0],'https://example.test');assert.equal(query.pathname,endpoint);assert.equal(query.searchParams.get('q'),'MRI knee');assert.equal(query.searchParams.has('procedure_code'),false);
  assert.match(host.innerHTML,/Service or care needed/);assert.doesNotMatch(host.innerHTML,/name="(?:procedure_code|code)"|Procedure code|CPT|HCPCS/);
 }
});

test('overview cost estimator submits a service description without a procedure code',async()=>{
 const paths=[],target=node(),form={...node(),values:{q:'blood test',city:'Philadelphia',price_type:'discounted_cash'}},nodes=new Map([['#overview-estimate',form]]);
 target.querySelector=selector=>{if(!nodes.has(selector))nodes.set(selector,node());return nodes.get(selector);};form.querySelector=()=>node();
 const h=harness(async path=>{paths.push(path);return {results:[]};},new Map([['#live-page-content',target]]));h.context.FormData=class{constructor(form){return Object.entries(form.values)[Symbol.iterator]();}};
 await h.run("activePage='overview';renderOverview()");await form.onsubmit({preventDefault(){},target:form});
 const query=new URL(paths.find(path=>path.startsWith('/prices/compare')),'https://example.test');assert.equal(query.searchParams.get('q'),'blood test');assert.equal(query.searchParams.has('procedure_code'),false);
 assert.match(target.innerHTML,/Service or care needed/);assert.doesNotMatch(target.innerHTML,/Procedure code|name="procedure_code"/);
});

test('procedure-code fields and sample codes are absent from the entire web interface',async()=>{
 for(const path of ['app.js','connected.mjs','integration.js','pages/overview.html','pages/care.html','pages/bill.html','pages/prescriptions.html']){
  const text=await readFile(new URL('../'+path,import.meta.url),'utf8');
  assert.doesNotMatch(text,/procedure[_ -]?code|\bCPT\b|\bHCPCS\b|Billing code|data-key="code"|name="code"|\$\{[^}]*field\('code'/i,path);
 }
});

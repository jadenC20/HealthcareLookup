import test from 'node:test';
import assert from 'node:assert/strict';
import http from 'node:http';
import {once} from 'node:events';
import {createFrontendServer} from '../server.mjs';
import {createApi,priceView} from '../api-client.mjs';

test('proxy preserves Go cookies, CSRF, query strings, multipart bytes, PDF and 501 responses', async t=>{
 const seen=[];
 const backend=http.createServer(async(req,res)=>{const chunks=[];for await(const chunk of req)chunks.push(chunk);seen.push({url:req.url,headers:req.headers,body:Buffer.concat(chunks).toString()});
 if(req.url==='/auth/login'){res.setHeader('Set-Cookie','clearcare_session=synthetic; HttpOnly; Path=/; SameSite=Strict');res.end(JSON.stringify({user:{id:'test'},csrf:'synthetic-csrf'}));}
 else if(req.url==='/documents/extract'){res.writeHead(501,{'Content-Type':'application/json'});res.end(JSON.stringify({detail:'Automatic document extraction is not available yet.'}));}
 else if(req.url==='/account/pdf'){res.writeHead(200,{'Content-Type':'application/pdf'});res.end('%PDF-1.4\nsynthetic');}
 else {res.setHeader('Content-Type','application/json');res.end(JSON.stringify({results:[]}));}});
 backend.listen(0,'127.0.0.1');await once(backend,'listening');
 const apiOrigin=`http://127.0.0.1:${backend.address().port}`;
 const publicAuth={configured:true,url:'https://synthetic.supabase.co',publishableKey:'sb_publishable_synthetic'};
 const frontend=createFrontendServer({backendUrl:apiOrigin,supabaseConfig:publicAuth,proxySecret:''});frontend.listen(0,'127.0.0.1');await once(frontend,'listening');
 t.after(()=>{frontend.closeAllConnections();frontend.close();backend.closeAllConnections();backend.close();});
 const origin=`http://127.0.0.1:${frontend.address().port}`;
 for(const asset of ['app.js','connected.mjs','insurance-view.mjs','api-client.mjs','supabase-auth.mjs','assets/vendor/supabase.js']){const response=await fetch(origin+'/'+asset);assert.equal(response.status,200,`${asset} must be served for the connected UI to load`);assert.match(response.headers.get('content-type'),/javascript/);}
 const authConfig=await fetch(origin+'/auth/config');assert.deepEqual(await authConfig.json(),publicAuth);
 const page=await fetch(origin+'/');assert.ok(page.headers.get('content-security-policy').includes("connect-src 'self' https://synthetic.supabase.co;"));
 const html=await page.text();assert.doesNotMatch(html,/data-page="account"/,'account access must not appear in primary navigation');assert.match(html,/data-account-label>Sign in</,'sign-in remains available as a utility action');
 const login=await fetch(origin+'/api/auth/login',{method:'POST',headers:{Origin:origin,'Content-Type':'application/json','X-ClearCare-Proxy-Secret':'browser-supplied'},body:'{"email":"synthetic@example.test","password":"test-password"}'});
 assert.match(login.headers.get('set-cookie'),/HttpOnly/);assert.equal((await login.json()).csrf,'synthetic-csrf');assert.equal(seen[0].headers.origin,apiOrigin);assert.equal(seen[0].headers['x-clearcare-proxy-secret'],undefined);
 const body=new FormData();body.append('file',new Blob(['%PDF-synthetic'],{type:'application/pdf'}),'synthetic.pdf');body.append('revision','revision-1');
 await fetch(origin+'/api/profile/insurance/sbc',{method:'POST',headers:{Origin:origin,Cookie:'clearcare_session=synthetic','X-CSRF-Token':'synthetic-csrf'},body});
 assert.equal(seen[1].headers['x-csrf-token'],'synthetic-csrf');assert.equal(seen[1].headers.cookie,'clearcare_session=synthetic');assert.match(seen[1].body,/%PDF-synthetic/);assert.match(seen[1].body,/revision-1/);assert.ok(seen[1].body.indexOf('name="file"')<seen[1].body.indexOf('name="revision"'));
 await fetch(origin+'/api/prices/compare?procedure_code=73721&city=New+York');assert.equal(seen[2].url,'/prices/compare?procedure_code=73721&city=New+York');
 const pdf=await fetch(origin+'/api/account/pdf');assert.equal(pdf.headers.get('content-type'),'application/pdf');assert.match(await pdf.text(),/^%PDF/);
 const unsupported=await fetch(origin+'/api/documents/extract',{method:'POST',headers:{Origin:origin}});assert.equal(unsupported.status,501);assert.match((await unsupported.json()).detail,/not available/);
 const before=seen.length;assert.equal((await fetch(origin+'/api/profile',{method:'PATCH',headers:{Origin:'https://untrusted.example'}})).status,403);assert.equal(seen.length,before);
 assert.equal((await fetch(origin+'/api/data/reload',{method:'POST'})).status,404);
 const invalidHostStatus=await new Promise((resolve,reject)=>{const request=http.get(origin+'/api/health',{headers:{Host:'untrusted.example'}},response=>{response.resume();resolve(response.statusCode);});request.on('error',reject);});assert.equal(invalidHostStatus,403);
});
test('hosted Node proxy sends the configured secret and public origin',async t=>{
 let received;
 const backend=http.createServer((req,res)=>{received=req.headers;res.end('{}');});backend.listen(0,'127.0.0.1');await once(backend,'listening');
 const frontend=createFrontendServer({backendUrl:`http://127.0.0.1:${backend.address().port}`,publicOrigin:'https://clearcare.example',proxySecret:'server-owned-secret',supabaseConfig:{configured:false}});frontend.listen(0,'127.0.0.1');await once(frontend,'listening');
 t.after(()=>{frontend.closeAllConnections();frontend.close();backend.closeAllConnections();backend.close();});
 const response=await fetch(`http://127.0.0.1:${frontend.address().port}/api/profile`,{method:'PATCH',headers:{Origin:'https://clearcare.example','X-ClearCare-Proxy-Secret':'browser-supplied',Forwarded:'host=untrusted.example','X-Forwarded-Host':'untrusted.example'},body:'{}'});
 assert.equal(response.status,200);await response.text();assert.equal(received.origin,'https://clearcare.example');assert.equal(received['x-clearcare-proxy-secret'],'server-owned-secret');assert.equal(received.forwarded,undefined);assert.equal(received['x-forwarded-host'],undefined);
});
test('client sends in-memory CSRF, handles Go JSON errors and clears expired sessions',async()=>{
 const calls=[];let unauthorized=0;
 const api=createApi({onUnauthorized:()=>unauthorized++,fetchImpl:async(url,options)=>{calls.push({url,...options});return new Response(JSON.stringify(url.endsWith('/auth/me')?{detail:'Please sign in first.'}:{ok:true}),{status:url.endsWith('/auth/me')?401:200});}});
 api.setSession({csrf:'test-csrf'});await api.request('/profile',{method:'PATCH',body:{name:'Test'}});assert.equal(calls[0].headers['X-CSRF-Token'],'test-csrf');assert.equal(calls[0].credentials,'same-origin');
 await assert.rejects(api.request('/auth/me'),/Please sign in first/);assert.equal(unauthorized,1);await api.request('/profile',{method:'PATCH',body:{name:'Test'}});assert.equal(calls[2].headers['X-CSRF-Token'],undefined);
});
test('unknown prices stay unknown and demo records are recognized across backend formats',()=>{assert.equal(priceView({price:null}).price,null);assert.equal(priceView({price:''}).price,null);assert.equal(priceView({price:0}).price,0);assert.equal(priceView({price:12,is_demo:true}).isDemo,true);assert.equal(priceView({is_demo:'true'}).isDemo,true);assert.equal(priceView({source_url:'DEMO'}).isDemo,true);assert.equal(priceView({price:12,is_demo:false}).isDemo,false);});
test('healthcare requests use fresh Supabase access tokens and public health checks do not',async()=>{
 let token='session-one';const seen=[];const api=createApi({getAccessToken:async()=>token,fetchImpl:async(path,options)=>{seen.push({path,...options});return new Response('{}');}});
 await api.request('/health');await api.request('/profile',{method:'PATCH',body:{plan:'My plan'}});token='session-two';await api.request('/bills/ai-review',{method:'POST',body:{ai_consent:true}});await api.request('/care/recommendations?procedure_code=73721');
 assert.equal(seen[0].headers.Authorization,undefined);assert.equal(seen[1].headers.Authorization,'Bearer session-one');assert.equal(seen[2].headers.Authorization,'Bearer session-two');assert.equal(seen[3].headers.Authorization,'Bearer session-two');
});

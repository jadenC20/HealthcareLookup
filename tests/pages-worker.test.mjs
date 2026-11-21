import test from 'node:test';
import assert from 'node:assert/strict';
import worker from '../deploy/pages-worker.mjs';

const frontend='https://clearcare.pages.dev';
const environment={CLEARCARE_API_URL:'https://clearcare-api.example',CLEARCARE_PROXY_SECRET:'worker-owned-secret',SUPABASE_URL:'https://synthetic.supabase.co',SUPABASE_PUBLISHABLE_KEY:'sb_publishable_synthetic'};

test('Pages proxy forwards authenticated API requests with its own secret and preserves upstream errors',async t=>{
 let received;
 const originalFetch=globalThis.fetch;
 globalThis.fetch=async(url,options)=>{received={url,...options};return new Response('{"detail":"Please retry."}',{status:429,headers:{'Content-Type':'application/json','Retry-After':'10'}});};
 t.after(()=>{globalThis.fetch=originalFetch;});
 const response=await worker.fetch(new Request(frontend+'/api/profile/insurance/retry?attempt=2',{method:'POST',headers:{Origin:frontend,Authorization:'Bearer user-session','X-ClearCare-Proxy-Secret':'browser-supplied',Forwarded:'host=untrusted.example','X-Forwarded-Host':'untrusted.example','Content-Type':'application/json'},body:'{"revision":"one"}'}),environment);
 assert.equal(received.url,'https://clearcare-api.example/profile/insurance/retry?attempt=2');
 assert.equal(received.headers.get('Authorization'),'Bearer user-session');
 assert.equal(received.headers.get('Origin'),frontend);
 assert.equal(received.headers.get('X-ClearCare-Proxy-Secret'),'worker-owned-secret');
 assert.equal(received.headers.get('Forwarded'),null);
 assert.equal(received.headers.get('X-Forwarded-Host'),null);
 assert.equal(received.redirect,'manual');
 assert.equal(response.status,429);assert.equal(response.headers.get('Retry-After'),'10');assert.equal(response.headers.get('Cache-Control'),'no-store');
 assert.deepEqual(await response.json(),{detail:'Please retry.'});
});

test('Pages proxy forwards medicine search JSON and backend validation errors',async t=>{
 const originalFetch=globalThis.fetch;let received;t.after(()=>{globalThis.fetch=originalFetch;});
 globalThis.fetch=async(url,options)=>{received={url,method:options.method,headers:options.headers,body:await new Response(options.body).text()};return new Response('{"detail":"Enter a medicine name."}',{status:422,headers:{'Content-Type':'application/json'}});};
 const body='{"query":"metformin","ai_consent":true}';
 const response=await worker.fetch(new Request(frontend+'/api/prescription-comparison/search',{method:'POST',headers:{Origin:frontend,'Content-Type':'application/json',Authorization:'Bearer user-session'},body}),environment);
 assert.equal(received.url,'https://clearcare-api.example/prescription-comparison/search');assert.equal(received.method,'POST');assert.equal(received.body,body);assert.equal(received.headers.get('Authorization'),'Bearer user-session');assert.equal(received.headers.get('X-ClearCare-Proxy-Secret'),'worker-owned-secret');
 assert.equal(response.status,422);assert.equal(response.headers.get('Cache-Control'),'no-store');assert.deepEqual(await response.json(),{detail:'Enter a medicine name.'});
});

test('Pages proxy rejects unsupported routes, cross-site requests, and missing server configuration before fetch',async t=>{
 let calls=0;const originalFetch=globalThis.fetch;globalThis.fetch=async()=>{calls++;throw Error('Should not fetch.');};t.after(()=>{globalThis.fetch=originalFetch;});
 assert.equal((await worker.fetch(new Request(frontend+'/api/data/reload',{method:'POST'}),environment)).status,404);
 assert.equal((await worker.fetch(new Request(frontend+'/api/profile',{headers:{Origin:'https://untrusted.example'}}),environment)).status,403);
 assert.equal((await worker.fetch(new Request(frontend+'/api/profile',{headers:{'Sec-Fetch-Site':'cross-site'}}),environment)).status,403);
 assert.equal((await worker.fetch(new Request(frontend+'/api/profile'),{...environment,CLEARCARE_PROXY_SECRET:''})).status,503);
 assert.equal(calls,0);
});

test('Pages auth configuration only exposes the Supabase public credential',async()=>{
 const response=await worker.fetch(new Request(frontend+'/auth/config'),{...environment,OPENROUTER_API_KEY:'synthetic-private-ai-key',SUPABASE_SERVICE_ROLE_KEY:'synthetic-private-storage-key'});
 assert.equal(response.status,200);assert.deepEqual(await response.json(),{configured:true,url:'https://synthetic.supabase.co',publishableKey:'sb_publishable_synthetic'});
 const invalid=await worker.fetch(new Request(frontend+'/auth/config'),{...environment,SUPABASE_PUBLISHABLE_KEY:'sb_secret_private'});
 assert.equal(invalid.status,503);
});

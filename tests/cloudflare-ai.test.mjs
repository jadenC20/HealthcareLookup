import test from 'node:test';
import assert from 'node:assert/strict';
import worker from '../deploy/pages-worker.mjs';

const secret='synthetic-internal-secret';
const endpoint='https://clearcare-ai.example/internal/ai';
const request=(payload,credential=secret)=>new Request(endpoint,{method:'POST',headers:{'Content-Type':'application/json','X-ClearCare-Proxy-Secret':credential},body:JSON.stringify(payload)});
const payload={instructions:'Return the requested JSON only.',input:[{role:'user',content:[{type:'input_text',text:'Synthetic public example'}]}],text:{format:{type:'json_object'}},max_output_tokens:200};
async function output(response){assert.equal(response.status,200);const result=await response.json();assert.equal(result.status,'completed');return result.output[0].content[0];}

test('private AI bridge rejects browsers and missing binding without running inference',async()=>{
 let calls=0;const env={CLEARCARE_PROXY_SECRET:secret,AI:{run:async()=>{calls++;}}};
 assert.equal((await worker.fetch(request(payload,''),env)).status,403);
 assert.equal((await worker.fetch(request(payload,'wrong'),env)).status,403);
 assert.equal((await worker.fetch(request(payload),{CLEARCARE_PROXY_SECRET:secret})).status,503);
 assert.equal(calls,0);
});

test('private AI bridge translates Responses text and images and never exposes its credential to AI',async()=>{
 let seen;const env={CLEARCARE_PROXY_SECRET:secret,AI:{run:async(model,options)=>{seen={model,options};return {response:{details:{insurer:'Synthetic'}}};}}};
 const image='data:image/png;base64,aW1hZ2U=';
 const part=await output(await worker.fetch(request({...payload,model:'untrusted-expensive-model',input:[{role:'user',content:[{type:'input_text',text:'Read this card'},{type:'input_image',image_url:image}]}]}),env));
 assert.deepEqual(JSON.parse(part.text),{details:{insurer:'Synthetic'}});
 assert.equal(seen.model,'@cf/meta/llama-4-scout-17b-16e-instruct');assert.equal(seen.options.response_format.type,'json_object');assert.equal(seen.options.max_tokens,200);
 assert.ok(seen.options.messages[1].content.some(part=>part.type==='image_url'&&part.image_url.url===image));assert.ok(!JSON.stringify(seen).includes(secret));
});

test('PDF conversion preserves page evidence and withholds invented citations',async()=>{
 let converted;const env={CLEARCARE_PROXY_SECRET:secret,AI:{toMarkdown:async(file,options)=>{converted={file,options};return {format:'markdown',data:'## Page 2\nDeductible: $500 individual.'};},run:async()=>({response:{benefits:[{name:'deductible',value:'$500 individual',page:2,quote:'Deductible: $500 individual.'},{name:'hospital_stay',value:'Invented',page:1,quote:'Invented quote'}],missing_fields:[]}})}};
 const pdf='data:application/pdf;base64,'+btoa('%PDF-synthetic');
 const part=await output(await worker.fetch(request({...payload,input:[{role:'user',content:[{type:'input_file',file_data:pdf,filename:'private-name.pdf'}]}]}),env));
 const data=JSON.parse(part.text);assert.equal(converted.file.name,'document.pdf');assert.equal(converted.file.blob.type,'application/pdf');assert.equal(converted.options.conversionOptions.pdf.metadata,true);
 assert.equal(data.benefits.length,1);assert.equal(data.benefits[0].page,2);assert.ok(data.missing_fields.includes('hospital_stay'));
});

test('PDF conversion without locations does not create page 1 citations',async()=>{
 const env={CLEARCARE_PROXY_SECRET:secret,AI:{toMarkdown:async()=>({format:'markdown',data:'Deductible: $500 individual.'}),run:async()=>({response:{benefits:[{name:'deductible',value:'$500 individual',page:1,quote:'Deductible: $500 individual.'}],missing_fields:[]}})}};
 const part=await output(await worker.fetch(request({...payload,input:[{role:'user',content:[{type:'input_file',file_data:'data:application/pdf;base64,'+btoa('%PDF-synthetic')}]}]}),env));
 assert.deepEqual(JSON.parse(part.text).benefits,[]);assert.ok(JSON.parse(part.text).missing_fields.includes('deductible'));
});

test('search supplies retrieved public evidence and only real source annotations',async t=>{
 const originalFetch=globalThis.fetch;let query,options;t.after(()=>{globalThis.fetch=originalFetch;});
 globalThis.fetch=async url=>{query=url;return new Response('<rss><channel><item><title>Plan SBC</title><link>https://insurer.example/real.pdf</link><description>2026 benefits &amp; coverage</description></item></channel></rss>');};
 const env={CLEARCARE_PROXY_SECRET:secret,AI:{run:async(_,input)=>{options=input;return {response:'Found https://insurer.example/real.pdf and https://invented.example/fake.pdf'};}}};
 const part=await output(await worker.fetch(request({instructions:'Find Summary of Benefits and Coverage documents.',input:'{"insurer":"Synthetic","year":2026}',tools:[{type:'web_search'}]}),env));
 assert.ok(query.startsWith('https://www.bing.com/search?'));assert.ok(decodeURIComponent(query).includes('Synthetic 2026'));assert.deepEqual(part.annotations,[{type:'url_citation',url:'https://insurer.example/real.pdf'}]);assert.ok(!part.text.includes('invented.example'));assert.match(options.messages.at(-1).content,/UNTRUSTED PUBLIC SEARCH EVIDENCE/);
});

test('failed search still runs AI and never fabricates source citations',async t=>{
 const originalFetch=globalThis.fetch;t.after(()=>{globalThis.fetch=originalFetch;});globalThis.fetch=async()=>new Response('Unavailable',{status:503});let calls=0;
 const env={CLEARCARE_PROXY_SECRET:secret,AI:{run:async(_,options)=>{calls++;assert.match(options.messages.at(-1).content,/No search sources/);return {response:'No supported sources were found.'};}}};
 const part=await output(await worker.fetch(request({input:'Synthetic public plan',tools:[{type:'web_search'}]}),env));assert.equal(calls,1);assert.deepEqual(part.annotations,[]);
});

test('quota and malformed model output return bounded errors',async()=>{
 const env={CLEARCARE_PROXY_SECRET:secret,AI:{run:async()=>{throw Error('daily neuron quota reached');}}};assert.equal((await worker.fetch(request(payload),env)).status,429);
 env.AI.run=async()=>({response:'not JSON'});assert.equal((await worker.fetch(request(payload),env)).status,502);
 assert.equal((await worker.fetch(request({...payload,input:[{role:'user',content:[{type:'input_image',image_url:'https://private.example/card.png'}]}]}),env)).status,400);
});

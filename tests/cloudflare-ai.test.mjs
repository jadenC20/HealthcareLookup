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

test('structured extraction sends required fields to Cloudflare constrained generation',async()=>{
 const schema={type:'object',properties:{details:{type:'object',properties:{insurer:{type:'string'},year:{type:'integer'}},required:['insurer','year'],additionalProperties:false},unreadable_fields:{type:'array',items:{type:'string'}}},required:['details','unreadable_fields'],additionalProperties:false};
 let received;
 const env={CLEARCARE_PROXY_SECRET:secret,AI:{run:async(_,options)=>{received=options.response_format;return {response:{details:{insurer:'Example',year:2026},unreadable_fields:[]}};}}};
 const part=await output(await worker.fetch(request({...payload,text:{format:{type:'json_schema',name:'insurance_card',strict:true,schema}}}),env));
 assert.deepEqual(received,{type:'json_schema',json_schema:schema});
 assert.deepEqual(JSON.parse(part.text),{details:{insurer:'Example',year:2026},unreadable_fields:[]});
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

test('private search response preserves retrieved snippets independently of model claims',async t=>{
 const originalFetch=globalThis.fetch;t.after(()=>{globalThis.fetch=originalFetch;});
 globalThis.fetch=async()=>new Response('<rss><channel><item><title>Metformin pharmacy offer</title><link>https://pharmacy.example/metformin</link><description>Metformin 500 mg, 30 tablets: $12 cash. Eligibility restrictions apply.</description></item></channel></rss>');
 const env={CLEARCARE_PROXY_SECRET:secret,AI:{run:async()=>({response:'Metformin is $1 at https://pharmacy.example/metformin'})}};
 const response=await worker.fetch(request({input:'Find published metformin pharmacy prices.',tools:[{type:'web_search'}]}),env);
 assert.equal(response.status,200);const data=await response.json();
 assert.deepEqual(data.search_results,[{url:'https://pharmacy.example/metformin',title:'Metformin pharmacy offer',excerpt:'Metformin 500 mg, 30 tablets: $12 cash. Eligibility restrictions apply.'}]);
 assert.match(data.output[0].content[0].text,/\$1 at/);assert.ok(!data.search_results[0].excerpt.includes('$1 at'));
 assert.deepEqual(data.search_diagnostics,[{engine:'bing_rss',http_status:200,result_count:1,error_kind:''}]);
});

test('private search diagnostics distinguish HTTP failures, challenges and network errors without query leakage',async t=>{
 const originalFetch=globalThis.fetch;t.after(()=>{globalThis.fetch=originalFetch;});let calls=0;
 globalThis.fetch=async()=>{calls++;if(calls===1)return new Response('Blocked',{status:403});if(calls===2)return new Response('<form id="challenge-form">Verify you are human</form>',{status:202});throw Error('Failed to fetch PRIVATE_MEDICINE_QUERY');};
 const env={CLEARCARE_PROXY_SECRET:secret,AI:{run:async()=>({response:'No supported sources.'})}};
 const response=await worker.fetch(request({input:'PRIVATE_MEDICINE_QUERY',tools:[{type:'web_search'}]}),env);assert.equal(response.status,200);const data=await response.json();
 assert.deepEqual(data.search_diagnostics,[{engine:'bing_rss',http_status:403,result_count:0,error_kind:'http_error'},{engine:'duckduckgo_html',http_status:202,result_count:0,error_kind:'challenge'},{engine:'duckduckgo_lite',http_status:0,result_count:0,error_kind:'network_error',error_name:'Error'}]);
 assert.ok(!JSON.stringify(data.search_diagnostics).includes('PRIVATE_MEDICINE_QUERY'));assert.ok(!JSON.stringify(data.search_diagnostics).includes(secret));
});

test('search works without AbortSignal.timeout, follows a safe 301, and clears its native timer',async t=>{
 const originalFetch=globalThis.fetch,timeoutDescriptor=Object.getOwnPropertyDescriptor(AbortSignal,'timeout'),originalSetTimeout=globalThis.setTimeout,originalClearTimeout=globalThis.clearTimeout,timers=[],cleared=[];
 Object.defineProperty(AbortSignal,'timeout',{value:undefined,configurable:true,writable:true});
 globalThis.setTimeout=(callback,delay,...args)=>{const timer=originalSetTimeout(callback,delay,...args);timers.push({timer,delay});return timer;};globalThis.clearTimeout=timer=>{cleared.push(timer);return originalClearTimeout(timer);};
 t.after(()=>{globalThis.fetch=originalFetch;if(timeoutDescriptor)Object.defineProperty(AbortSignal,'timeout',timeoutDescriptor);else delete AbortSignal.timeout;globalThis.setTimeout=originalSetTimeout;globalThis.clearTimeout=originalClearTimeout;});
 const calls=[],document='https://pharmacy.example/metformin';
 globalThis.fetch=async(url,options)=>{calls.push({url,options});return calls.length===1?new Response(null,{status:301,headers:{Location:'https://bing.com/search?format=rss&q=metformin'}}):new Response('<rss><channel><item><title>Metformin prices</title><link>'+document+'</link><description>Published pharmacy details.</description></item></channel></rss>');};
 const env={CLEARCARE_PROXY_SECRET:secret,AI:{run:async()=>({response:document})}};
 const response=await worker.fetch(request({input:'metformin',tools:[{type:'web_search'}]}),env);assert.equal(response.status,200);const data=await response.json();
 assert.equal(calls.length,2);assert.ok(calls.every(call=>call.options.redirect==='manual'&&call.options.signal instanceof AbortSignal));assert.equal(calls[1].url,'https://bing.com/search?format=rss&q=metformin');
 assert.deepEqual(data.search_results,[{url:document,title:'Metformin prices',excerpt:'Published pharmacy details.'}]);assert.equal(timers.length,1);assert.ok(timers[0].delay>0&&timers[0].delay<=6000);assert.ok(cleared.includes(timers[0].timer));
});

test('search never follows redirects outside the HTTPS search-engine allowlist',async t=>{
 const originalFetch=globalThis.fetch;t.after(()=>{globalThis.fetch=originalFetch;});let calls=0;
 const locations=['https://private.example/collect','http://www.bing.com/search','https://secret@duckduckgo.com/'];
 globalThis.fetch=async()=>new Response(null,{status:302,headers:{Location:locations[calls++]}});
 const env={CLEARCARE_PROXY_SECRET:secret,AI:{run:async()=>({response:'No supported sources.'})}};
 const response=await worker.fetch(request({input:'metformin',tools:[{type:'web_search'}]}),env);const data=await response.json();assert.equal(calls,3);assert.deepEqual(data.search_results,[]);
 assert.ok(data.search_diagnostics.every(item=>item.http_status===302&&item.error_kind==='redirect_blocked'&&item.error_name==='Error'));
});

test('search bounds redirect loops and clears timers on fetch failures',async t=>{
 const originalFetch=globalThis.fetch,originalSetTimeout=globalThis.setTimeout,originalClearTimeout=globalThis.clearTimeout,timers=[],cleared=[];let calls=0;
 globalThis.setTimeout=(callback,delay,...args)=>{const timer=originalSetTimeout(callback,delay,...args);timers.push(timer);return timer;};globalThis.clearTimeout=timer=>{cleared.push(timer);return originalClearTimeout(timer);};
 t.after(()=>{globalThis.fetch=originalFetch;globalThis.setTimeout=originalSetTimeout;globalThis.clearTimeout=originalClearTimeout;});
 globalThis.fetch=async()=>{calls++;return new Response(null,{status:307,headers:{Location:'https://www.bing.com/search?loop='+calls}});};
 const env={CLEARCARE_PROXY_SECRET:secret,AI:{run:async()=>({response:'No supported sources.'})}};
 const response=await worker.fetch(request({input:'metformin',tools:[{type:'web_search'}]}),env);const data=await response.json();assert.equal(calls,9);assert.ok(data.search_diagnostics.every(item=>item.error_kind==='redirect_limit'));assert.equal(timers.length,3);assert.ok(timers.every(timer=>cleared.includes(timer)));
});

test('SBC search falls back to DuckDuckGo HTML and decodes the actual PDF URL',async t=>{
 const originalFetch=globalThis.fetch,queries=[];t.after(()=>{globalThis.fetch=originalFetch;});
 const document='https://insurer.example/coverage/2026/sbc.pdf?variant=Silver&state=NY';
 globalThis.fetch=async url=>{queries.push(url);if(url.includes('bing.com'))return new Response('Unavailable',{status:503});return new Response('<div class="result"><a rel="nofollow" class="result__a" href="//duckduckgo.com/l/?uddg='+encodeURIComponent(document)+'&amp;rut=tracking">Official SBC</a><a class="result__snippet">Individual plan benefits &amp; coverage.</a></div>');};
 const env={CLEARCARE_PROXY_SECRET:secret,AI:{run:async()=>({response:'Document: '+document})}};
 const part=await output(await worker.fetch(request({instructions:'Find Summary of Benefits and Coverage documents.',input:JSON.stringify({insurer:'Synthetic',plan_name:'Long plan display label',plan_id:'12345NY0010001',network:'Unhelpful network label',employer:'An unrelated employer',state:'NY',year:2026}),tools:[{type:'web_search'}]}),env));
 assert.equal(queries.length,2);assert.ok(queries[1].startsWith('https://html.duckduckgo.com/html/'));const query=new URL(queries[1]).searchParams.get('q');assert.match(query,/Synthetic 12345NY0010001 2026 SBC/);assert.doesNotMatch(query,/Long plan|Unhelpful|unrelated/);
 assert.deepEqual(part.annotations,[{type:'url_citation',url:document}]);
});

test('Bing HTML returned in place of RSS still yields its exact decoded source',async t=>{
 const originalFetch=globalThis.fetch;t.after(()=>{globalThis.fetch=originalFetch;});let calls=0;
 const document='https://state.example.gov/benefits/sbc.pdf';
 globalThis.fetch=async()=>{calls++;return new Response('<li class="b_algo"><h2><a target="_blank" href="https://www.bing.com/ck/a?u=a1'+btoa(document).replace(/=/g,'')+'&amp;ntb=1">State benefit document</a></h2><p>Summary of benefits for 2026.</p></li>');};
 const env={CLEARCARE_PROXY_SECRET:secret,AI:{run:async()=>({response:document})}};
 const part=await output(await worker.fetch(request({instructions:'Find Summary of Benefits and Coverage documents.',input:'Synthetic plan 2026',tools:[{type:'web_search'}]}),env));
 assert.equal(calls,1);assert.deepEqual(part.annotations,[{type:'url_citation',url:document}]);
});

test('DuckDuckGo Lite is tried after a challenge page and excludes navigation and unsafe URLs',async t=>{
 const originalFetch=globalThis.fetch,queries=[];t.after(()=>{globalThis.fetch=originalFetch;});
 const document='https://insurer.example/sbc-2026.pdf';
 globalThis.fetch=async url=>{queries.push(url);if(url.includes('bing.com'))return new Response('<rss><channel></channel></rss>');if(url.includes('html.duckduckgo'))return new Response('<h1>Please solve this challenge</h1>');return new Response('<a href="https://duckduckgo.com/settings" class="result-link">Navigation</a><a href="javascript:alert(1)" class="result-link">Unsafe</a><a href="https://secret@insurer.example/private.pdf" class="result-link">Credentials</a><tr><td><a rel="nofollow" href="'+document+'" class=\'result-link\'>Official plan SBC</a></td></tr><tr><td class=\'result-snippet\'>2026 plan coverage.</td></tr>');};
 let evidence;const env={CLEARCARE_PROXY_SECRET:secret,AI:{run:async(_,options)=>{evidence=options.messages.at(-1).content;return {response:document};}}};
 const part=await output(await worker.fetch(request({instructions:'Find Summary of Benefits and Coverage documents.',input:[{role:'user',content:[{type:'input_text',text:'Synthetic plan 2026'}]}],tools:[{type:'web_search'}]}),env));
 assert.equal(queries.length,3);assert.ok(queries[2].startsWith('https://lite.duckduckgo.com/lite/'));assert.deepEqual(part.annotations,[{type:'url_citation',url:document}]);assert.match(evidence,/2026 plan coverage/);assert.doesNotMatch(evidence,/javascript:|secret@|duckduckgo.com\/settings/);
});

test('SBC discovery keeps looking past landing pages and prioritizes a retrieved PDF',async t=>{
 const originalFetch=globalThis.fetch;let calls=0;t.after(()=>{globalThis.fetch=originalFetch;});
 globalThis.fetch=async()=>{calls++;return calls===1?new Response('<rss><channel><item><title>Insurer documents</title><link>https://insurer.example/documents</link><description>Find plan coverage.</description></item></channel></rss>'):new Response('<a class="result__a" href="https://insurer.example/plan.pdf">2026 SBC</a>');};
 const env={CLEARCARE_PROXY_SECRET:secret,AI:{run:async()=>({response:'https://insurer.example/plan.pdf'})}};
 const part=await output(await worker.fetch(request({instructions:'Find Summary of Benefits and Coverage documents.',input:'Synthetic plan 2026',tools:[{type:'web_search'}]}),env));
 assert.equal(calls,2);assert.deepEqual(part.annotations.map(row=>row.url),['https://insurer.example/plan.pdf','https://insurer.example/documents']);
});

test('medicine price search keeps looking past drug facts and prioritizes a retrieved pharmacy price page',async t=>{
 const originalFetch=globalThis.fetch,queries=[];t.after(()=>{globalThis.fetch=originalFetch;});
 globalThis.fetch=async url=>{queries.push(url);return url.includes('bing.com')?new Response('<rss><channel><item><title>Metformin uses and side effects</title><link>https://www.drugs.com/metformin.html</link><description>Drug information and precautions.</description></item><item><title>Metformin oral route</title><link>https://www.mayoclinic.org/drugs-supplements/metformin-oral-route/description/drg-20067074</link><description>Medical information.</description></item></channel></rss>'):new Response('<a class="result__a" href="https://www.goodrx.com/metformin">Metformin prices and coupons</a><a class="result__snippet">Compare cash pharmacy prices and coupon restrictions.</a>');};
 const env={CLEARCARE_PROXY_SECRET:secret,AI:{run:async()=>({response:'See https://www.goodrx.com/metformin'})}};
 const response=await worker.fetch(request({input:'metformin pharmacy cash prices coupons',tools:[{type:'web_search'}]}),env);assert.equal(response.status,200);const data=await response.json();
 assert.equal(queries.length,2);assert.ok(queries[1].startsWith('https://html.duckduckgo.com/html/'));assert.equal(data.search_results[0].url,'https://www.goodrx.com/metformin');assert.equal(data.search_results[1].url,'https://www.drugs.com/metformin.html');assert.equal(data.search_diagnostics[1].result_count,1);
});

test('medicine price search distinguishes Drugs.com facts from its actual price-guide path',async t=>{
 const originalFetch=globalThis.fetch;let calls=0;t.after(()=>{globalThis.fetch=originalFetch;});
 globalThis.fetch=async()=>{calls++;return new Response('<rss><channel><item><title>Metformin drug information</title><link>https://www.drugs.com/metformin.html</link><description>Uses and precautions.</description></item><item><title>Metformin prices</title><link>https://www.drugs.com/price-guide/metformin</link><description>Published prices, coupons and limitations.</description></item></channel></rss>');};
 const env={CLEARCARE_PROXY_SECRET:secret,AI:{run:async()=>({response:'See https://www.drugs.com/price-guide/metformin'})}};
 const response=await worker.fetch(request({input:'metformin medicine prices',tools:[{type:'web_search'}]}),env);const data=await response.json();assert.equal(calls,1);assert.equal(data.search_results[0].url,'https://www.drugs.com/price-guide/metformin');
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

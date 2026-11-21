const apiPaths=/^(?:\/health|\/sample|\/providers|\/prices\/compare|\/data\/status|\/bills\/(?:analyze|ai-review)|\/bill-review\/(?:extract|analyze|observations)|\/care\/(?:recommendations|search)|\/care-search\/recommend|\/prescription-comparison\/(?:drugs|compare)|\/jobs(?:\/[a-zA-Z0-9-]+)?|\/results(?:\/[a-zA-Z0-9-]+)?|\/documents\/extract|\/auth\/(?:register|login|me|logout)|\/profile(?:\/preferences|\/insurance-card(?:\/file)?|\/insurance(?:\/(?:details|retry|select|sbc|context))?)?|\/account(?:\/pdf)?|\/reviews(?:\/[a-zA-Z0-9-]+(?:\/pdf)?)?|\/reports\/pdf)$/;
const json=(status,detail)=>Response.json({detail},{status,headers:{'Cache-Control':'no-store'}});
function origin(value){const url=new URL(value);if(url.protocol!=='https:'||url.username||url.password||url.pathname!=='/'||url.search||url.hash)throw Error('Expected HTTPS origin');return url;}
export default {
 async fetch(request,env){
  const url=new URL(request.url);
  if(url.pathname==='/auth/config'){
   if(request.method!=='GET')return json(405,'Use GET.');
   try{const supabase=origin(env.SUPABASE_URL);const key=env.SUPABASE_PUBLISHABLE_KEY||'';if(!/^sb_publishable_[A-Za-z0-9_-]+$/.test(key))return json(503,'Configure a Supabase publishable key.');return Response.json({configured:true,url:supabase.origin,publishableKey:key},{headers:{'Cache-Control':'no-store'}});}catch{return json(503,'Supabase authentication is not configured.');}
  }
  if(url.pathname.startsWith('/api/')){
   const apiPath=url.pathname.slice(4);
   if(!apiPaths.test(apiPath))return json(404,'Unsupported API endpoint.');
   if(request.headers.get('sec-fetch-site')==='cross-site'||request.headers.has('Origin')&&request.headers.get('Origin')!==url.origin)return json(403,'Origin not allowed.');
   if(Number(request.headers.get('Content-Length')||0)>21*1024*1024)return json(413,'Request exceeds 21 MB.');
   let backend;try{backend=origin(env.CLEARCARE_API_URL);}catch{return json(503,'Backend origin is not configured.');}
   if(!env.CLEARCARE_PROXY_SECRET)return json(503,'API proxy is not configured.');
   const headers=new Headers(request.headers);
   headers.delete('Host');headers.delete('Connection');headers.delete('Forwarded');headers.delete('X-Forwarded-Host');
   headers.set('Origin',url.origin);headers.set('X-ClearCare-Proxy-Secret',env.CLEARCARE_PROXY_SECRET);
   try{
    const upstream=await fetch(backend.origin+apiPath+url.search,{method:request.method,headers,body:['GET','HEAD'].includes(request.method)?undefined:request.body,redirect:'manual'});
    const responseHeaders=new Headers(upstream.headers);responseHeaders.set('Cache-Control','no-store');responseHeaders.set('X-Content-Type-Options','nosniff');
    return new Response(upstream.body,{status:upstream.status,headers:responseHeaders});
   }catch{return json(502,'Healthcare backend is unavailable. Please retry shortly.');}
  }
  if(!['GET','HEAD'].includes(request.method))return json(405,'Use GET.');
  const asset=await env.ASSETS.fetch(request);
  const headers=new Headers(asset.headers);
  headers.set('X-Content-Type-Options','nosniff');headers.set('Referrer-Policy','same-origin');
  let authOrigin='';try{authOrigin=origin(env.SUPABASE_URL).origin;}catch{}
  headers.set('Content-Security-Policy',`default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; font-src 'self'; img-src 'self' blob: data:; connect-src 'self' ${authOrigin}; object-src 'none'; base-uri 'self'; frame-ancestors 'none'`);
  return new Response(asset.body,{status:asset.status,headers});
 }
};

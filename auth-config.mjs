// Only these two public values may be sent to the browser.
export function publicSupabaseConfig(env=process.env){
 const url=env.SUPABASE_URL?.trim()||'',key=(env.SUPABASE_PUBLISHABLE_KEY||env.SUPABASE_ANON_KEY||'').trim();
 if(!url||!key)return {configured:false,detail:'Supabase setup is needed. Set SUPABASE_URL and SUPABASE_PUBLISHABLE_KEY in the frontend server environment, then restart it.'};
 try{const parsed=new URL(url);if(parsed.username||parsed.password||parsed.pathname!=='/'||parsed.search||parsed.hash||!(parsed.protocol==='https:'||parsed.protocol==='http:'&&['localhost','127.0.0.1'].includes(parsed.hostname)))throw Error();}catch{return {configured:false,detail:'SUPABASE_URL must be your HTTPS project origin (or a local Supabase origin).'};}
 let publicKey=/^sb_publishable_[A-Za-z0-9_-]+$/.test(key);
 if(!publicKey){try{publicKey=JSON.parse(Buffer.from(key.split('.')[1],'base64url').toString()).role==='anon';}catch{}}
 if(!publicKey||key.startsWith('sb_secret_'))return {configured:false,detail:'Use a Supabase publishable key or legacy anon key. Secret and service_role keys cannot be used by the frontend.'};
 return {configured:true,url:new URL(url).origin,publishableKey:key};
}

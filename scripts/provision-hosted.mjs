// Run only after account authorization. State includes secrets and stays in
// ignored tmp/, with owner-only permissions. Never print command errors that
// could include environment arguments or database credentials.
import {execFileSync} from 'node:child_process';
import {chmodSync,existsSync,mkdirSync,readFileSync,writeFileSync} from 'node:fs';
import {randomBytes} from 'node:crypto';
import {loadEnvFile} from 'node:process';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
process.chdir(root);
if(existsSync('.env'))loadEnvFile('.env');
mkdirSync('tmp',{recursive:true,mode:0o700});
chmodSync('tmp',0o700);
const file='tmp/hosted-state.json';
const state=existsSync(file)?JSON.parse(readFileSync(file,'utf8')):{role:'clearcare_runtime_'+randomBytes(4).toString('hex'),password:randomBytes(32).toString('hex'),proxySecret:randomBytes(32).toString('hex')};
const save=()=>{writeFileSync(file,JSON.stringify(state),{mode:0o600});chmodSync(file,0o600);};save();
const run=(cmd,args)=>execFileSync(cmd,args,{encoding:'utf8',maxBuffer:4*1024*1024,stdio:['ignore','pipe','pipe']});
const supabase=args=>run('npx',['--yes','supabase@latest',...args]);
const project='dkagkbfjwnaezncgspfb';
async function api(url,method,body,headers){const r=await fetch(url,{method,headers:{'Content-Type':'application/json',...headers},body:body===undefined?undefined:JSON.stringify(body)});if(!r.ok)throw Error('Hosted configuration request failed: '+r.status);return r.status===204?null:r.json();}
function renderToken(){
 const fromEnv=process.env.RENDER_API_KEY?.trim();if(fromEnv)return fromEnv;
 const configFile=path.join(process.env.HOME,'.render','cli.yaml');
 if(!existsSync(configFile))throw Error('Render API authorization unavailable. Set RENDER_API_KEY before updating an existing backend.');
 const value=readFileSync(configFile,'utf8').match(/^\s+key:\s*(.+)\s*$/m)?.[1]?.trim();
 if(!value)throw Error('Render API authorization unavailable.');
 try{return value.startsWith('"')?JSON.parse(value):value.startsWith("'")?value.slice(1,-1).replaceAll("''", "'"):value;}catch{throw Error('Render API authorization unavailable.');}
}
try{
 const keys=JSON.parse(supabase(['projects','api-keys','--remote='+project,'--reveal','--output-format','json'])).keys;
 const serviceKey=keys.find(k=>k.name==='service_role')?.api_key;
 if(!serviceKey)throw Error('Private storage key unavailable');
 const auth={apikey:serviceKey,Authorization:'Bearer '+serviceKey};
 const base='https://'+project+'.supabase.co';
 const bucket=await fetch(base+'/storage/v1/bucket/clearcare-private',{headers:auth});
 const bucketInfo=await bucket.json();
 if(bucket.status===404||String(bucketInfo.statusCode)==='404')await api(base+'/storage/v1/bucket','POST',{id:'clearcare-private',name:'clearcare-private',public:false,file_size_limit:10485760,allowed_mime_types:['application/pdf','image/png','image/jpeg']},auth);
 else if(!bucket.ok||bucketInfo.public)throw Error('Storage bucket unavailable or not private ('+bucket.status+')');
 console.log('Private storage bucket ready.');
 if(!state.databaseReady){
  const sql=`BEGIN; CREATE ROLE ${state.role} LOGIN PASSWORD '${state.password}' CONNECTION LIMIT 5; CREATE SCHEMA IF NOT EXISTS clearcare; GRANT USAGE,CREATE ON SCHEMA clearcare TO ${state.role}; GRANT ALL ON ALL TABLES IN SCHEMA clearcare TO ${state.role}; GRANT ALL ON ALL SEQUENCES IN SCHEMA clearcare TO ${state.role}; COMMIT;`;
  writeFileSync('tmp/provision-database.sql',sql,{mode:0o600});
  chmodSync('tmp/provision-database.sql',0o600);
  supabase(['db','query','--remote='+project,'--file','tmp/provision-database.sql','--yes','--output-format','json']);
  state.databaseReady=true;save();
 }
 const pooler=new URL(readFileSync('supabase/.temp/pooler-url','utf8').trim());
 pooler.username=state.role+'.'+project;pooler.password=state.password;pooler.search='?sslmode=require';
 const env={CLEARCARE_HOST:'0.0.0.0',CLEARCARE_STORE:'postgres',DATABASE_URL:pooler.href,CLEARCARE_ALLOWED_ORIGINS:'https://clearcare.pages.dev',CLEARCARE_PROXY_SECRET:state.proxySecret,CLEARCARE_SECURE_COOKIES:'1',SUPABASE_URL:base,SUPABASE_PUBLISHABLE_KEY:process.env.SUPABASE_PUBLISHABLE_KEY,CLEARCARE_DOCUMENT_STORE:'supabase',SUPABASE_STORAGE_BUCKET:'clearcare-private',SUPABASE_SERVICE_ROLE_KEY:serviceKey};
 if(!env.SUPABASE_PUBLISHABLE_KEY)throw Error('Publishable auth key unavailable');
 env.CLEARCARE_AI_PROVIDER='cloudflare';
 env.CLOUDFLARE_AI_MODEL=process.env.CLOUDFLARE_AI_MODEL?.trim()||'@cf/meta/llama-4-scout-17b-16e-instruct';
 env.CLOUDFLARE_AI_URL=process.env.CLOUDFLARE_AI_URL?.trim()||'https://clearcare-ai.agrigine-mcp.workers.dev/internal/ai';
 const existing=!!state.render;
 if(!state.render){
  const args=['services','create','--name','clearcare-api','--type','web_service','--runtime','go','--plan','free','--region','ohio','--repo','https://github.com/jadenC20/HealthcareLookup','--branch','main','--root-directory','backend/goapp','--build-command','go build -o bin/clearcare .','--start-command','./bin/clearcare','--health-check-path','/health','--auto-deploy=false','--confirm','-o','json'];
  for(const[k,v]of Object.entries(env))args.push('--env-var',k+'='+v);
  state.render=JSON.parse(run('render',args));save();
 }
 const service=state.render.service||state.render;
 const id=service.id||service.service?.id;
 const backend=service.serviceDetails?.url||service.url||service.service?.serviceDetails?.url;
 if(!id||!backend)throw Error('Render service created; inspect metadata for service ID and origin');
 if(existing){
  const token=renderToken();
  // Render's bulk env update replaces omitted variables. Updating each key
  // keeps unrelated settings and existing credentials intact.
  for(const[key,value]of Object.entries(env))await api('https://api.render.com/v1/services/'+encodeURIComponent(id)+'/env-vars/'+encodeURIComponent(key),'PUT',{value},{Authorization:'Bearer '+token});
 }
 const config=readFileSync(path.join(process.env.HOME,'Library/Preferences/.wrangler/config/default.toml'),'utf8');
 const token=config.match(/^oauth_token\s*=\s*"([^"]+)"/m)?.[1];
 if(!token)throw Error('Cloudflare authorization unavailable');
 await api('https://api.cloudflare.com/client/v4/accounts/c7edb7f34a4d4f8e504fc5b08da5dbe4/pages/projects/clearcare','PATCH',{deployment_configs:{production:{env_vars:Object.fromEntries(Object.entries({CLEARCARE_API_URL:backend,CLEARCARE_PROXY_SECRET:state.proxySecret,SUPABASE_URL:base,SUPABASE_PUBLISHABLE_KEY:env.SUPABASE_PUBLISHABLE_KEY}).map(([key,value])=>[key,{type:key==='CLEARCARE_PROXY_SECRET'?'secret_text':'plain_text',value}]))}}},{Authorization:'Bearer '+token});
 console.log('Hosted configuration saved. Render service: '+id+'; origin: '+backend);
}catch(error){console.error('Provisioning stopped. '+(error.status?'An authenticated CLI command failed.':error.message.startsWith('Command failed')?'An authenticated CLI command failed.':error.message));process.exitCode=1;}

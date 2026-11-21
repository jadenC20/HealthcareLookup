import test from 'node:test';
import assert from 'node:assert/strict';
import {publicSupabaseConfig} from '../auth-config.mjs';
import {createSupabaseAuth} from '../supabase-auth.mjs';
const config={configured:true,url:'https://example.supabase.co',publishableKey:'sb_publishable_synthetic'};
test('only public Supabase configuration is exposed and elevated keys are rejected',()=>{
 const env={SUPABASE_URL:config.url,SUPABASE_PUBLISHABLE_KEY:config.publishableKey,OPENAI_API_KEY:'never-expose',SUPABASE_SERVICE_ROLE_KEY:'never-expose'};
 assert.deepEqual(publicSupabaseConfig(env),config);
 assert.equal(publicSupabaseConfig({...env,SUPABASE_PUBLISHABLE_KEY:'sb_secret_no'}).configured,false);
 const jwt=role=>'header.'+Buffer.from(JSON.stringify({role})).toString('base64url')+'.signature';
 assert.equal(publicSupabaseConfig({...env,SUPABASE_PUBLISHABLE_KEY:jwt('service_role')}).configured,false);
 assert.equal(publicSupabaseConfig({...env,SUPABASE_PUBLISHABLE_KEY:jwt('anon')}).configured,true);
 assert.equal(publicSupabaseConfig({...env,SUPABASE_URL:'https://user:password@example.com'}).configured,false);
 assert.equal(publicSupabaseConfig({...env,SUPABASE_URL:'http://untrusted.example'}).configured,false);
});
test('signup confirmation is not mistaken for an authenticated session; passwords only reach the SDK',async()=>{
 const calls=[],changes=[];let eventCallback;
 const auth={onAuthStateChange:callback=>{eventCallback=callback;return {data:{subscription:{unsubscribe(){}}}};},getSession:async()=>({data:{session:null},error:null}),signUp:async value=>{calls.push(['signup',value]);return {data:{user:{id:'pending'},session:null},error:null};},signInWithPassword:async value=>{calls.push(['login',value]);const session={user:{id:'confirmed',email:value.email}};eventCallback('SIGNED_IN',session);return {data:{session,user:session.user},error:null};},signOut:async()=>({data:{},error:null}),stopAutoRefresh(){}};
 const client=createSupabaseAuth({createClient:(url,key,options)=>{calls.push(['initialize',url,key,options]);return {auth};},fetchImpl:async path=>{assert.equal(path,'/auth/config');return new Response(JSON.stringify(config));},onChange:user=>changes.push(user)});
 await client.initialize();
 const pending=await client.signUp({email:'synthetic@example.test',password:'not-a-real-password',name:'Test',redirectTo:'http://127.0.0.1:4182/#account'});
 assert.equal(pending.session,null);assert.equal(client.user,null);assert.equal(calls[1][1].options.data.display_name,'Test');
 await client.signIn({email:'synthetic@example.test',password:'not-a-real-password'});assert.equal(client.user.id,'confirmed');
 assert.equal(calls[0][3].auth.flowType,'pkce');await client.signOut();assert.equal(client.user,null);assert.equal(changes.at(-1),null);
});
test('Supabase errors surface and an unconfigured app never tries to sign up',async()=>{
 const client=createSupabaseAuth({fetchImpl:async()=>new Response(JSON.stringify({configured:false,detail:'Set the public key.'}))});await client.initialize();await assert.rejects(client.signUp({}),/Set the public key/);
 const failure=createSupabaseAuth({fetchImpl:async()=>new Response(JSON.stringify(config)),createClient:()=>({auth:{onAuthStateChange:()=>({data:{subscription:{unsubscribe(){}}}}),getSession:async()=>({data:{session:null}}),signInWithPassword:async()=>({error:{message:'Email not confirmed'}})}})});
 await failure.initialize();await assert.rejects(failure.signIn({email:'synthetic@example.test',password:'invalid'}),/Email not confirmed/);
});
test('an initial session response cannot restore an account after a newer sign-out event',async()=>{
 let eventCallback,resolveSession;const changes=[];
 const client=createSupabaseAuth({fetchImpl:async()=>new Response(JSON.stringify(config)),createClient:()=>({auth:{onAuthStateChange:callback=>{eventCallback=callback;return {data:{subscription:{unsubscribe(){}}}};},getSession:()=>new Promise(resolve=>{resolveSession=resolve;})}}),onChange:user=>changes.push(user)});
 const initializing=client.initialize();while(!resolveSession)await new Promise(setImmediate);
 eventCallback('SIGNED_OUT',null);resolveSession({data:{session:{user:{id:'stale-account'}}}});await initializing;
 assert.equal(client.user,null);assert.deepEqual(changes,[null]);
});

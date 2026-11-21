// Passwords go only to Supabase. Go receives the access token for verification.
export function createSupabaseAuth({createClient,fetchImpl=globalThis.fetch,onChange=()=>{}}={}){
 let client=null,currentUser=null,configuration={configured:false,detail:'Checking Supabase configuration…'};
 let subscription;
 const requireClient=()=>{if(!client)throw new Error(configuration.detail||'Supabase is not configured.');return client;};
 const unwrap=result=>{if(result.error)throw new Error(result.error.message||'Supabase could not complete this request.');return result.data;};
 return {
  get config(){return configuration;},get user(){return currentUser;},
  async initialize(){
   try{const response=await fetchImpl('/auth/config',{cache:'no-store'});if(!response.ok)throw new Error('Unable to load Supabase configuration. Restart the frontend server.');configuration=await response.json();
    if(!configuration.configured)return;
    if(typeof createClient!=='function')throw new Error('The Supabase SDK did not load. Refresh the page or restart the frontend server.');
    client=createClient(configuration.url,configuration.publishableKey,{auth:{persistSession:true,autoRefreshToken:true,detectSessionInUrl:true,flowType:'pkce',storageKey:'clearcare-supabase-auth'}});
    subscription=client.auth.onAuthStateChange((_event,session)=>{currentUser=session?.user||null;onChange(currentUser);}).data.subscription;
    const data=unwrap(await client.auth.getSession());currentUser=data.session?.user||null;onChange(currentUser);
   }catch(error){configuration={configured:false,detail:error.message};throw error;}
  },
  async signUp({email,password,name,redirectTo}){return unwrap(await requireClient().auth.signUp({email,password,options:{data:{display_name:name},emailRedirectTo:redirectTo}}));},
  async getAccessToken(){if(!client)return null;const data=unwrap(await client.auth.getSession());return data.session?.access_token||null;},
  async signIn({email,password}){return unwrap(await requireClient().auth.signInWithPassword({email,password}));},
  async resendConfirmation(email,redirectTo){return unwrap(await requireClient().auth.resend({type:'signup',email,options:{emailRedirectTo:redirectTo}}));},
  async updateName(name){return unwrap(await requireClient().auth.updateUser({data:{display_name:name}}));},
  async signOut(){unwrap(await requireClient().auth.signOut({scope:'local'}));currentUser=null;onChange(null);},
  dispose(){subscription?.unsubscribe();client?.auth.stopAutoRefresh();}
 };
}

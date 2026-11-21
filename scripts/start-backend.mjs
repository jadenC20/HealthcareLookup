// Set OPENROUTER_API_KEY in .env or the backend terminal. It stays in the process
// environment and is inherited by Go; never printed or sent to the frontend.
import {loadEnvFile} from 'node:process';
import {existsSync} from 'node:fs';
import {spawn} from 'node:child_process';
import {fileURLToPath} from 'node:url';
const root=new URL('../',import.meta.url);
const envFile=new URL('.env',root);if(existsSync(envFile))loadEnvFile(fileURLToPath(envFile));
const fallback='C:\\Program Files\\Go\\bin\\go.exe';
const go=process.platform==='win32'&&existsSync(fallback)?fallback:'go';
const child=spawn(go,['run','.'],{cwd:fileURLToPath(new URL('backend/goapp/',root)),env:process.env,stdio:'inherit',windowsHide:true});
child.on('error',error=>{console.error('Could not start Go:',error.message);process.exitCode=1;});
child.on('exit',code=>{process.exitCode=code??1;});
process.on('SIGINT',()=>child.kill('SIGINT'));

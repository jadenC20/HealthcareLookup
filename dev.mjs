import {spawn} from 'node:child_process';
import {existsSync} from 'node:fs';
const python=process.env.CLEARCARE_PYTHON || (process.platform==='win32'?'.venv/Scripts/python.exe':'.venv/bin/python');
if(!existsSync(python)){console.error('Create .venv and install backend/requirements.txt first. See README.md.');process.exit(1)}
const env={...process.env,...(process.argv.includes('--demo')?{CLEARCARE_DEMO:'1'}:{})};
const children=[spawn(python,['-m','uvicorn','main:app','--app-dir','backend','--host','127.0.0.1','--port','8000'],{stdio:'inherit',env}),spawn(process.execPath,['server.mjs'],{stdio:'inherit',env})];
let stopping=false;
function stop(code=0){if(stopping)return;stopping=true;children.forEach(c=>c.kill());process.exitCode=code}
children.forEach(c=>{c.on('error',e=>{console.error(e.message);stop(1)});c.on('exit',code=>stop(code||0))});
process.on('SIGINT',()=>stop());process.on('SIGTERM',()=>stop());

import {spawn} from 'node:child_process';
import {tmpdir} from 'node:os';
import path from 'node:path';
import {unlink} from 'node:fs/promises';
const go=process.env.CLEARCARE_GO || 'go';
const binary=path.join(tmpdir(),`clearcare-backend-${process.pid}${process.platform==='win32'?'.exe':''}`);
const env={...process.env,...(process.argv.includes('--demo')?{CLEARCARE_DEMO:'1'}:{})};
const build=spawn(go,['build','-o',binary,'.'],{cwd:'backend/goapp',stdio:'inherit',env});
let stopping=false, children=[];
function stop(code=0){if(stopping)return;stopping=true;build.kill();children.forEach(c=>c.kill());unlink(binary).catch(()=>{});process.exitCode=code}
build.on('error',e=>{console.error(`Unable to start Go: ${e.message}. Install Go 1.24+ or set CLEARCARE_GO.`);stop(1)});
build.on('exit',code=>{if(stopping)return;if(code!==0){stop(code||1);return}children=[spawn(binary,[],{cwd:'backend/goapp',stdio:'inherit',env}),spawn(process.execPath,['server.mjs'],{stdio:'inherit',env})];children.forEach(c=>{c.on('error',e=>{console.error(e.message);stop(1)});c.on('exit',code=>stop(code||0))})});
process.on('SIGINT',()=>stop());process.on('SIGTERM',()=>stop());

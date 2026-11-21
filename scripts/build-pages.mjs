import {mkdir,copyFile,cp,writeFile,rm} from 'node:fs/promises';
import {fileURLToPath} from 'node:url';
import path from 'node:path';

const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
const out=path.join(root,'dist');
// Remove stale deployment files, including anything left by a prior build.
await rm(out,{recursive:true,force:true});
await mkdir(out,{recursive:true});
// Explicit public asset list: never upload the repository root or .env.
for(const file of ['index.html','styles.css','connected.css','app.js','connected.mjs','api-client.mjs','supabase-auth.mjs','insurance-view.mjs'])await copyFile(path.join(root,file),path.join(out,file));
for(const dir of ['assets','pages'])await cp(path.join(root,dir),path.join(out,dir),{recursive:true});
await copyFile(path.join(root,'deploy/pages-worker.mjs'),path.join(out,'_worker.js'));
await writeFile(path.join(out,'_routes.json'),JSON.stringify({version:1,include:['/*'],exclude:[]}));
console.log('Cloudflare Pages assets written to dist.');

import http from 'node:http';
import {readFile} from 'node:fs/promises';
import {fileURLToPath} from 'node:url';
import path from 'node:path';
const root=path.dirname(fileURLToPath(import.meta.url));
const allowed=new Map([['/','index.html'],['/index.html','index.html'],['/styles.css','styles.css'],['/app.js','app.js']]);
const types={'.html':'text/html; charset=utf-8','.css':'text/css; charset=utf-8','.js':'text/javascript; charset=utf-8'};
http.createServer(async(req,res)=>{const file=allowed.get(new URL(req.url,'http://localhost').pathname);if(!file){res.writeHead(404);res.end('Not found');return}try{const data=await readFile(path.join(root,file));res.writeHead(200,{'Content-Type':types[path.extname(file)],'Cache-Control':'no-store'});res.end(data)}catch{res.writeHead(500);res.end('Unable to read asset')}}).listen(4173,'127.0.0.1',()=>console.log('ClearCare preview: http://127.0.0.1:4173'));

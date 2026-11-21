import http from 'node:http';
import {readFile} from 'node:fs/promises';
import {fileURLToPath} from 'node:url';
import path from 'node:path';
const root=path.dirname(fileURLToPath(import.meta.url));
const allowed=new Map([['/','index.html'],['/index.html','index.html'],['/styles.css','styles.css'],['/app.js','app.js']]);
const types={'.html':'text/html; charset=utf-8','.css':'text/css; charset=utf-8','.js':'text/javascript; charset=utf-8'};
allowed.set('/integration.js','integration.js');
http.createServer(async(req,res)=>{
 const pathname=new URL(req.url,'http://localhost').pathname;
 if(pathname.startsWith('/api/')){
  const upstream=http.request({hostname:'127.0.0.1',port:8000,path:req.url.slice(4),method:req.method,headers:{...req.headers,host:'127.0.0.1:8000'}},r=>{res.writeHead(r.statusCode,r.headers);r.pipe(res)});
  upstream.on('error',()=>{if(!res.headersSent)res.writeHead(502,{'Content-Type':'application/json'});res.end(JSON.stringify({detail:'Backend unavailable. Start the Go backend on port 8000.'}))});
  req.pipe(upstream);return;
 }
 const file=allowed.get(pathname);if(!file){res.writeHead(404);res.end('Not found');return}try{const data=await readFile(path.join(root,file));res.writeHead(200,{'Content-Type':types[path.extname(file)],'Cache-Control':'no-store','X-Content-Type-Options':'nosniff'});res.end(data)}catch{res.writeHead(500);res.end('Unable to read asset')}
}).listen(Number(process.env.PORT||4173),'127.0.0.1',()=>console.log('ClearCare preview: http://127.0.0.1:'+(process.env.PORT||4173)));

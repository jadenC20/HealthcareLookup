import http from 'node:http';
import {readFile} from 'node:fs/promises';
import {fileURLToPath} from 'node:url';
import path from 'node:path';
const root=path.dirname(fileURLToPath(import.meta.url));
const allowed=new Set(['/index.html','/app.js','/styles.css','/pages/overview.html','/pages/bill.html','/pages/care.html','/pages/prescriptions.html','/assets/map.svg','/assets/material-symbols-outlined.woff2',...['literata','nunito-sans'].flatMap(font=>[400,600,700].map(weight=>`/assets/${font}-latin-${weight}-normal.woff2`))]);
const types={'.html':'text/html; charset=utf-8','.js':'text/javascript; charset=utf-8','.css':'text/css; charset=utf-8','.woff2':'font/woff2','.svg':'image/svg+xml'};
const port=Number(process.env.PORT||4180);
http.createServer(async(req,res)=>{const pathname=new URL(req.url,'http://localhost').pathname;const file=pathname==='/'?'/index.html':pathname;if(!allowed.has(file)){res.writeHead(404,{'Content-Type':'text/plain'});res.end('Not found');return}try{const data=await readFile(path.join(root,file));res.writeHead(200,{'Content-Type':types[path.extname(file)],'Cache-Control':'no-store','X-Content-Type-Options':'nosniff','Content-Security-Policy':"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; font-src 'self'; img-src 'self' blob: data:; connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'"});res.end(data)}catch{res.writeHead(500);res.end('Unable to read asset')}}).listen(port,'127.0.0.1',()=>console.log(`ClearCare Terra: http://127.0.0.1:${port}`));

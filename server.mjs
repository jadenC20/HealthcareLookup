import {publicSupabaseConfig} from './auth-config.mjs';
import {existsSync} from 'node:fs';
import {loadEnvFile} from 'node:process';
import http from 'node:http';
import https from 'node:https';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
const root = path.dirname(fileURLToPath(import.meta.url));
const allowed = new Set(['/index.html', '/app.js', '/connected.mjs', '/supabase-auth.mjs', '/assets/vendor/supabase.js', '/assets/vendor/supabase-LICENSE.txt', '/insurance-view.mjs', '/api-client.mjs', '/styles.css', '/connected.css', '/pages/overview.html', '/pages/bill.html', '/pages/care.html', '/pages/prescriptions.html', '/assets/map.svg', '/assets/material-symbols-outlined.woff2', ...['literata', 'nunito-sans'].flatMap(font => [400, 600, 700].map(weight => `/assets/${font}-latin-${weight}-normal.woff2`))]);
const types = { '.html': 'text/html; charset=utf-8', '.js': 'text/javascript; charset=utf-8', '.mjs': 'text/javascript; charset=utf-8', '.css': 'text/css; charset=utf-8', '.woff2': 'font/woff2', '.svg': 'image/svg+xml' };
export const apiPaths = /^(?:\/health|\/sample|\/providers|\/prices\/compare|\/data\/status|\/bills\/(?:analyze|ai-review)|\/bill-review\/(?:extract|analyze|observations)|\/care\/(?:recommendations|search)|\/care-search\/recommend|\/prescription-comparison\/(?:drugs|compare)|\/jobs(?:\/[a-zA-Z0-9-]+)?|\/results(?:\/[a-zA-Z0-9-]+)?|\/documents\/extract|\/auth\/(?:register|login|me|logout)|\/profile(?:\/preferences|\/insurance-card(?:\/file)?|\/insurance(?:\/(?:details|retry|select|sbc|context))?)?|\/account(?:\/pdf)?|\/reviews(?:\/[a-zA-Z0-9-]+(?:\/pdf)?)?|\/reports\/pdf)$/;
export function createFrontendServer({ backendUrl = process.env.CLEARCARE_API_URL || 'http://127.0.0.1:8000', publicOrigin = process.env.CLEARCARE_PUBLIC_ORIGIN || '', assetRoot = root, supabaseConfig = publicSupabaseConfig() } = {}) {
  const target = new URL(backendUrl);
  const publicUrl = publicOrigin ? new URL(publicOrigin) : null;
  if (publicUrl && (publicUrl.protocol !== 'https:' || publicUrl.username || publicUrl.password || publicUrl.pathname !== '/' || publicUrl.search || publicUrl.hash)) throw new Error('CLEARCARE_PUBLIC_ORIGIN must be an HTTPS origin without credentials or a path.');
  const authConfig = supabaseConfig.configured ? publicSupabaseConfig({SUPABASE_URL:supabaseConfig.url,SUPABASE_PUBLISHABLE_KEY:supabaseConfig.publishableKey}) : supabaseConfig;
  if (!['http:', 'https:'].includes(target.protocol) || target.username || target.password || target.pathname !== '/' || target.search || target.hash) throw new Error('CLEARCARE_API_URL must be an HTTP(S) origin without credentials or a path.');
  const jsonError = (res, status, detail) => { if (res.headersSent) { res.destroy(); return; } res.writeHead(status, { 'Content-Type': 'application/json', 'Cache-Control': 'no-store' }); res.end(JSON.stringify({ detail })); };
  const server = http.createServer(async (req, res) => {
    const requested = new URL(req.url, 'http://localhost');
    if (requested.pathname === '/auth/config') { if(req.method!=='GET'){jsonError(res,405,'Use GET.');return;} res.writeHead(200,{'Content-Type':'application/json','Cache-Control':'no-store'});res.end(JSON.stringify(authConfig));return;}
    if (requested.pathname.startsWith('/api/')) {
      const apiPath = requested.pathname.slice(4);
      if (!apiPaths.test(apiPath)) { jsonError(res, 404, 'No supported API endpoint matches this action.'); return; }
      const port = server.address().port;
      const validHosts = [`127.0.0.1:${port}`, `localhost:${port}`];
      const validOrigins = validHosts.map(host => `http://${host}`);
      if (publicUrl) { validHosts.push(publicUrl.host); validOrigins.push(publicUrl.origin); }
      const origin = req.headers.origin;
      if (!validHosts.includes(req.headers.host) || (origin && !validOrigins.includes(origin)) || req.headers['sec-fetch-site'] === 'cross-site') { jsonError(res, 403, 'Origin not allowed.'); return; }
      if (Number(req.headers['content-length'] || 0) > 21 * 1024 * 1024) { jsonError(res, 413, 'Request exceeds 21 MB.'); return; }
      // Validate the browser origin first; translate it to the Go API's own allowed origin.
      const headers = { ...req.headers, host: target.host };
      if (origin) headers.origin = target.origin;
      delete headers.connection;
      const upstream = (target.protocol === 'https:' ? https : http).request({ hostname: target.hostname, port: target.port, protocol: target.protocol, path: apiPath + requested.search, method: req.method, headers }, response => {
        res.writeHead(response.statusCode, { ...response.headers, 'Cache-Control': 'no-store', 'X-Content-Type-Options': 'nosniff' }); response.pipe(res);
        response.on('error', () => res.destroy());
      });
      let bytes = 0;
      req.on('data', chunk => { bytes += chunk.length; if (bytes > 21 * 1024 * 1024) { jsonError(res, 413, 'Request exceeds 21 MB.'); upstream.destroy(); } });
      req.on('aborted', () => upstream.destroy());
      res.on('close', () => { if (!res.writableEnded) upstream.destroy(); });
      upstream.setTimeout(300_000, () => { jsonError(res, 504, 'Backend timed out. Reload insurance status before retrying.'); upstream.destroy(); });
      upstream.on('error', () => { if (!res.writableEnded) jsonError(res, 502, 'Go backend unavailable. Start it on port 8000, or configure CLEARCARE_API_URL.'); });
      req.pipe(upstream); return;
    }
    const file = requested.pathname === '/' ? '/index.html' : requested.pathname;
    if (!allowed.has(file)) { res.writeHead(404, { 'Content-Type': 'text/plain' }); res.end('Not found'); return; }
    try { const data = await readFile(path.join(assetRoot, file)); res.writeHead(200, { 'Content-Type': types[path.extname(file)], 'Cache-Control': 'no-store', 'X-Content-Type-Options': 'nosniff', 'Content-Security-Policy': `default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; font-src 'self'; img-src 'self' blob: data:; connect-src 'self' ${authConfig.configured ? new URL(authConfig.url).origin : ''}; object-src 'none'; base-uri 'self'; frame-ancestors 'none'` }); res.end(data); }
    catch { res.writeHead(500); res.end('Unable to read asset'); }
  });
  return server;
}
if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const envFile=path.join(root,'.env');if(existsSync(envFile))loadEnvFile(envFile);
  const port = Number(process.env.PORT || 4180);
  createFrontendServer().listen(port, '127.0.0.1', () => console.log(`ClearCare: http://127.0.0.1:${port}`));
}

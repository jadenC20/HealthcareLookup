// ClearCare MCP server for Cloudflare Workers.
// Stateless MCP Streamable HTTP (JSON responses): initialize, ping,
// tools/list, tools/call, resources/list|read, prompts/list|get.
// Every tool forwards to one fixed backend route with the caller's own
// Supabase bearer token, so backend per-user ownership checks still apply.
import { tools, toolByName } from './tools.mjs';

export const PROTOCOL_VERSIONS = ['2025-11-25', '2025-06-18', '2025-03-26'];
const SERVER = { name: 'clearcare', title: 'ClearCare Healthcare', version: '1.0.0' };
const MAX_FILE = 10 * 1024 * 1024;
const WAIT_MS = 50_000;

const cors = (env, request) => {
  const origin = request.headers.get('Origin');
  const allowed = (env.MCP_ALLOWED_ORIGINS || '').split(',').map((v) => v.trim()).filter(Boolean);
  const headers = { 'Cache-Control': 'no-store', 'X-Content-Type-Options': 'nosniff' };
  if (origin && allowed.includes(origin)) {
    Object.assign(headers, {
      'Access-Control-Allow-Origin': origin, Vary: 'Origin',
      'Access-Control-Allow-Headers': 'Authorization, Content-Type, Accept, MCP-Protocol-Version, Mcp-Session-Id',
      'Access-Control-Allow-Methods': 'POST, OPTIONS', 'Access-Control-Expose-Headers': 'MCP-Protocol-Version',
    });
  }
  return headers;
};
const rpcResult = (id, result) => ({ jsonrpc: '2.0', id, result });
const rpcError = (id, code, message, data) => ({ jsonrpc: '2.0', id: id ?? null, error: { code, message, ...(data ? { data } : {}) } });
const text = (value) => ({ type: 'text', text: typeof value === 'string' ? value : JSON.stringify(value, null, 2) });

function backendOrigin(env) {
  const url = new URL(env.CLEARCARE_API_URL);
  const local = ['localhost', '127.0.0.1'].includes(url.hostname) && env.MCP_ALLOW_HTTP_BACKEND === '1';
  if ((url.protocol !== 'https:' && !local) || url.username || url.password || url.pathname !== '/' || url.search) throw Error('CLEARCARE_API_URL must be an HTTPS origin.');
  return url.origin;
}

function bearer(request) {
  const parts = (request.headers.get('Authorization') || '').trim().split(/\s+/);
  return parts.length === 2 && /^bearer$/i.test(parts[0]) && parts[1].length <= 16384 ? parts[1] : '';
}

function decodeBase64(value, label) {
  if (typeof value !== 'string' || !/^[A-Za-z0-9+/\r\n]*={0,2}$/.test(value)) throw toolError(`${label} must be base64.`);
  const binary = atob(value.replace(/\s+/g, ''));
  if (binary.length > MAX_FILE) throw toolError(`${label} exceeds 10 MB.`);
  return Uint8Array.from(binary, (c) => c.charCodeAt(0));
}
const toolError = (message) => Object.assign(Error(message), { tool: true });

function resolvePath(tool, args) {
  return tool.path.replace(/\{(\w+)\}/g, (_, key) => {
    const value = args[key];
    if (typeof value !== 'string' || !/^[A-Za-z0-9_-]{1,128}$/.test(value)) throw toolError(`Invalid ${key}.`);
    return encodeURIComponent(value);
  });
}

function buildRequest(tool, args) {
  const path = resolvePath(tool, args);
  const query = new URLSearchParams();
  for (const key of tool.query || []) if (args[key] !== undefined && args[key] !== '') query.set(key, String(args[key]));
  let body;
  const headers = {};
  if (tool.body === 'json') {
    const payload = { ...args };
    for (const key of ['wait', ...(tool.path.match(/\{(\w+)\}/g) || []).map((k) => k.slice(1, -1))]) delete payload[key];
    body = JSON.stringify(payload);
    headers['Content-Type'] = 'application/json';
  } else if (tool.body === 'multipart') {
    body = new FormData();
    for (const [field, prefix] of Object.entries(tool.files)) {
      const data = args[`${prefix}file_base64`];
      if (data === undefined) continue;
      const type = args[`${prefix}content_type`] || 'application/pdf';
      if (!['application/pdf', 'image/png', 'image/jpeg'].includes(type)) throw toolError('Files must be PDF, PNG or JPEG.');
      const name = String(args[`${prefix}filename`] || `${field}.${type.split('/')[1]}`).replace(/[^\w. -]/g, '_').slice(0, 120);
      body.append(field, new Blob([decodeBase64(data, field)], { type }), name);
    }
    for (const key of tool.fields || []) if (args[key] !== undefined) body.append(key, String(args[key]));
  }
  return { path: path + (query.size ? `?${query}` : ''), body, headers };
}

async function callBackend(env, token, method, path, { body, headers = {} } = {}) {
  const h = new Headers(headers);
  h.set('Accept', 'application/json, application/pdf, */*');
  if (token) h.set('Authorization', `Bearer ${token}`);
  if (env.CLEARCARE_PROXY_SECRET) h.set('X-ClearCare-Proxy-Secret', env.CLEARCARE_PROXY_SECRET);
  const target = backendOrigin(env) + path;
  const response = await (env.BACKEND?.fetch ? env.BACKEND.fetch(target, { method, headers: h, body }) : fetch(target, { method, headers: h, body, redirect: 'manual' }));
  return response;
}

async function readBackend(response, binary) {
  const type = response.headers.get('Content-Type') || '';
  if (binary && response.ok && !type.includes('json')) {
    const bytes = new Uint8Array(await response.arrayBuffer());
    let raw = '';
    for (let i = 0; i < bytes.length; i += 0x8000) raw += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
    return { binary: { mimeType: type.split(';')[0] || 'application/octet-stream', blob: btoa(raw), size: bytes.length } };
  }
  const raw = await response.text();
  try { return { json: raw ? JSON.parse(raw) : {} }; } catch { return { json: { detail: raw.slice(0, 2000) } }; }
}

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

async function waitForJob(env, token, job, deadline) {
  let current = job;
  while (current && ['queued', 'running'].includes(current.status) && Date.now() < deadline) {
    await sleep(1500);
    const response = await callBackend(env, token, 'GET', `/jobs/${encodeURIComponent(current.id)}`);
    const { json } = await readBackend(response, false);
    if (!response.ok) return { job: current, poll_error: json?.detail || response.status };
    current = json.job || json;
  }
  return current;
}

export async function callTool(env, token, name, args = {}) {
  const tool = toolByName.get(name);
  if (!tool) throw toolError(`Unknown tool: ${name}`);
  if (tool.auth !== false && !token) {
    return { isError: true, content: [text('Sign-in required: connect with a ClearCare (Supabase) access token in the Authorization: Bearer header.')] };
  }
  if (tool.confirm && args.confirm !== tool.confirm) return { isError: true, content: [text(`Refused: pass confirm="${tool.confirm}" to proceed.`)] };
  const request = buildRequest(tool, args);
  const response = await callBackend(env, token, tool.method, request.path, request);
  const out = await readBackend(response, tool.binary);
  if (out.binary) {
    return { content: [text(`Downloaded ${out.binary.mimeType}, ${out.binary.size} bytes.`), { type: 'resource', resource: { uri: `clearcare://download${request.path}`, mimeType: out.binary.mimeType, blob: out.binary.blob } }] };
  }
  let data = out.json;
  const wantsWait = args.wait !== false;
  if (response.ok && wantsWait && (tool.async || tool.pollable)) {
    const job = data.job || (tool.pollable ? data : null);
    if (job?.id) {
      const finished = await waitForJob(env, token, job, Date.now() + WAIT_MS);
      data = finished?.status === 'completed'
        ? { job_id: finished.id, status: 'completed', result_id: finished.result_id, result: finished.result }
        : { job: finished, note: 'Still running. Call clearcare_get_job with this id to continue waiting.' };
    }
  }
  if (!response.ok) return { isError: true, content: [text({ status: response.status, ...data })] };
  return { content: [text(data)], structuredContent: data && typeof data === 'object' && !Array.isArray(data) ? data : { value: data } };
}

const resources = [
  { uri: 'clearcare://profile', name: 'profile', title: 'Your profile', mimeType: 'application/json', path: '/auth/me' },
  { uri: 'clearcare://preferences', name: 'preferences', title: 'Your preferences', mimeType: 'application/json', path: '/profile/preferences' },
  { uri: 'clearcare://insurance', name: 'insurance', title: 'Your insurance status', mimeType: 'application/json', path: '/profile/insurance' },
  { uri: 'clearcare://reviews', name: 'reviews', title: 'Saved bill reviews', mimeType: 'application/json', path: '/reviews' },
  { uri: 'clearcare://results', name: 'results', title: 'Saved results', mimeType: 'application/json', path: '/results' },
  { uri: 'clearcare://jobs', name: 'jobs', title: 'Background jobs', mimeType: 'application/json', path: '/jobs' },
  { uri: 'clearcare://health', name: 'health', title: 'Service health', mimeType: 'application/json', path: '/health' },
];
const prompts = [
  { name: 'cheapest_medicine_near_me', title: 'Cheapest medicine near me', description: 'Find the lowest price for a medicine at a nearby pharmacy or by mail order.', arguments: [{ name: 'medicine', required: true }, { name: 'zip', required: false }],
    render: (a) => `Use clearcare_find_medicine_prices for "${a.medicine}"${a.zip ? ` with zip ${a.zip}` : ' (use my profile location)'}. Summarize the suggestions: the best nearby pharmacy option, the best delivery option, and the cheapest verified price. Quote sources and remind me to confirm with the pharmacy.` },
  { name: 'review_my_bill', title: 'Review my bill', description: 'Check a medical bill for errors and overcharges.', arguments: [],
    render: () => 'Help me review a medical bill with ClearCare. If I attach a file, use clearcare_extract_bill, confirm the lines with me, then clearcare_analyze_bill. Explain findings plainly and draft an inquiry if useful.' },
  { name: 'find_care', title: 'Find care', description: 'Find a provider matching my preferences.', arguments: [{ name: 'need', required: true }],
    render: (a) => `Find care for: ${a.need}. Read my preferences with clearcare_get_preferences, then use clearcare_recommend_care. Note that network status and appointments are unverified.` },
];

async function handleRpc(env, token, message) {
  const { id, method, params = {} } = message;
  switch (method) {
    case 'initialize': {
      const requested = params.protocolVersion;
      return rpcResult(id, {
        protocolVersion: PROTOCOL_VERSIONS.includes(requested) ? requested : PROTOCOL_VERSIONS[0],
        capabilities: { tools: { listChanged: false }, resources: { listChanged: false }, prompts: { listChanged: false } },
        serverInfo: SERVER,
        instructions: 'ClearCare: healthcare bill review, insurance plan interpretation, care search and prescription price comparison. Private tools need the user\'s ClearCare access token as a Bearer header. Long-running tools wait for their job by default. Treat results as information to confirm with providers, pharmacies and insurers, not guarantees.',
      });
    }
    case 'ping': return rpcResult(id, {});
    case 'tools/list':
      return rpcResult(id, { tools: tools.map((t) => ({ name: t.name, title: t.title, description: t.description, inputSchema: t.input, annotations: { readOnlyHint: t.method === 'GET', destructiveHint: !!t.destructive, idempotentHint: ['GET', 'PUT', 'DELETE'].includes(t.method), openWorldHint: true } })) });
    case 'tools/call': {
      try { return rpcResult(id, await callTool(env, token, params.name, params.arguments || {})); }
      catch (error) {
        if (error.tool) return rpcResult(id, { isError: true, content: [text(error.message)] });
        if (/unknown tool/i.test(error.message)) return rpcError(id, -32602, error.message);
        return rpcResult(id, { isError: true, content: [text('ClearCare backend is unavailable. Please retry shortly.')] });
      }
    }
    case 'resources/list': return rpcResult(id, { resources: resources.map(({ path, ...r }) => r) });
    case 'resources/read': {
      const resource = resources.find((r) => r.uri === params.uri);
      if (!resource) return rpcError(id, -32002, 'Resource not found', { uri: params.uri });
      if (resource.path !== '/health' && !token) return rpcError(id, -32001, 'Sign-in required.');
      const response = await callBackend(env, token, 'GET', resource.path);
      const { json } = await readBackend(response, false);
      return rpcResult(id, { contents: [{ uri: resource.uri, mimeType: 'application/json', text: JSON.stringify(json, null, 2) }] });
    }
    case 'prompts/list': return rpcResult(id, { prompts: prompts.map(({ render, ...p }) => p) });
    case 'prompts/get': {
      const prompt = prompts.find((p) => p.name === params.name);
      if (!prompt) return rpcError(id, -32602, 'Unknown prompt');
      return rpcResult(id, { description: prompt.description, messages: [{ role: 'user', content: { type: 'text', text: prompt.render(params.arguments || {}) } }] });
    }
    default: return rpcError(id, -32601, `Method not found: ${method}`);
  }
}

export async function handleMcp(request, env) {
  const headers = cors(env, request);
  if (request.method === 'OPTIONS') return new Response(null, { status: 204, headers });
  const origin = request.headers.get('Origin');
  const allowed = (env.MCP_ALLOWED_ORIGINS || '').split(',').map((v) => v.trim()).filter(Boolean);
  if (origin && !allowed.includes(origin)) return Response.json(rpcError(null, -32000, 'Origin not allowed.'), { status: 403, headers });
  if (request.method === 'GET') return new Response('ClearCare MCP: POST JSON-RPC to this endpoint. SSE streams are not offered.', { status: 405, headers: { ...headers, Allow: 'POST' } });
  if (request.method === 'DELETE') return new Response(null, { status: 405, headers: { ...headers, Allow: 'POST' } });
  if (request.method !== 'POST') return new Response(null, { status: 405, headers });
  const version = request.headers.get('MCP-Protocol-Version');
  if (version && !PROTOCOL_VERSIONS.includes(version)) return Response.json(rpcError(null, -32600, `Unsupported MCP-Protocol-Version ${version}.`), { status: 400, headers });
  if (Number(request.headers.get('Content-Length') || 0) > 30 * 1024 * 1024) return Response.json(rpcError(null, -32600, 'Request too large.'), { status: 413, headers });
  let message;
  try { message = await request.json(); } catch { return Response.json(rpcError(null, -32700, 'Parse error'), { status: 400, headers }); }
  const token = bearer(request);
  const one = async (m) => {
    if (!m || m.jsonrpc !== '2.0' || typeof m.method !== 'string') return rpcError(m?.id, -32600, 'Invalid Request');
    if (m.id === undefined) return null; // notification
    return handleRpc(env, token, m);
  };
  const out = Array.isArray(message) ? (await Promise.all(message.map(one))).filter(Boolean) : await one(message);
  if (!out || (Array.isArray(out) && !out.length)) return new Response(null, { status: 202, headers });
  return Response.json(out, { headers: { ...headers, 'MCP-Protocol-Version': version || PROTOCOL_VERSIONS[0] } });
}

export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    if (url.pathname === '/mcp' || url.pathname === '/') return handleMcp(request, env);
    if (url.pathname === '/health') return Response.json({ status: 'ok', server: SERVER, tools: tools.length });
    return new Response('Not found', { status: 404 });
  },
};

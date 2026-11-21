// Go HTTP/JSON client. Sessions use HttpOnly cookies; never browser storage.
export class ApiError extends Error {
  constructor(message, status = 0) { super(message); this.name = 'ApiError'; this.status = status; }
}
export function createApi({ fetchImpl = globalThis.fetch, onUnauthorized = () => {}, getAccessToken = async () => null } = {}) {
  let csrf = '';
  async function request(path, { method = 'GET', body, binary = false, signal } = {}) {
    const headers = { Accept: binary ? '*/*' : 'application/json' };
    if (/^\/(?:auth\/me|profile|reviews|account|documents\/extract|bills\/ai-review|care\/(?:recommendations|search)|prescription-comparison|jobs|reports\/pdf)(?:[/?]|$)/.test(path)) {
      const accessToken = await getAccessToken();
      if (accessToken) headers.Authorization = 'Bearer ' + accessToken;
    }
    const multipart = typeof FormData !== 'undefined' && body instanceof FormData;
    if (body !== undefined && !multipart) headers['Content-Type'] = 'application/json';
    if (csrf && !['GET', 'HEAD'].includes(method)) headers['X-CSRF-Token'] = csrf;
    let response;
    try { response = await fetchImpl('/api' + path, { method, headers, credentials: 'same-origin', body: body === undefined ? undefined : multipart ? body : JSON.stringify(body), signal }); }
    catch (error) { if (error.name === 'AbortError') throw error; throw new ApiError('The healthcare API could not be reached. Please try again shortly.'); }
    if (!response.ok) {
      const text = await response.text(); let message;
      try { const data = JSON.parse(text); message = typeof data.detail === 'string' ? data.detail : undefined; } catch {}
      if (response.status === 401) { csrf = ''; onUnauthorized(); }
      throw new ApiError(message || (response.status === 501 ? 'This feature is not implemented by the backend yet.' : `Request failed (${response.status}). Please retry.`), response.status);
    }
    if (binary) return response.blob();
    try { return await response.json(); } catch { throw new ApiError('The backend returned an unreadable response.', response.status); }
  }
  return { request, setSession: data => { csrf = data?.csrf || ''; }, clearSession: () => { csrf = ''; } };
}
export function priceView(row) {
  const amount = row.price === null || row.price === undefined || row.price === '' ? NaN : Number(row.price);
  return { ...row, price: Number.isFinite(amount) ? amount : null, isDemo: row.is_demo === true || row.is_demo === 1 || row.is_demo === '1' || row.is_demo === 'true' || /^DEMO/i.test(row.source_url||''), insuranceStatus: row.insurance_status || 'unknown' };
}
export function validateUpload(file, pdfOnly = false) {
  if (!file) throw new ApiError('Choose a file first.');
  if (file.size > 10 * 1024 * 1024) throw new ApiError('Choose a file of 10 MB or smaller.');
  if (!(pdfOnly ? /\.pdf$/i : /\.(pdf|png|jpe?g)$/i).test(file.name)) throw new ApiError(pdfOnly ? 'Choose a PDF.' : 'Choose a PDF, PNG, or JPEG.');
  return file;
}

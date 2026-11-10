// Typed fetch wrapper: Romanian error messages, CSRF header on mutations.

export class ApiError extends Error {
  status: number;
  field?: string;
  constructor(status: number, message: string, field?: string) {
    super(message);
    this.status = status;
    this.field = field;
  }
}

const OFFLINE = 'Serverul nu răspunde.';
let csrf = '';
let onUnauthorized: (() => void) | null = null;

export function setCsrf(token: string): void {
  csrf = token;
}

/** Called when an admin request answers 401 (expired session). */
export function setUnauthorizedHandler(fn: (() => void) | null): void {
  onUnauthorized = fn;
}

export type Params = Record<string, string | number | undefined>;

export function withQuery(path: string, params?: Params): string {
  if (!params) return path;
  const q = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== '') q.set(k, String(v));
  }
  const s = q.toString();
  return s ? `${path}?${s}` : path;
}

async function request<T>(method: string, url: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json' };
  const init: RequestInit = { method, headers, credentials: 'same-origin' };
  if (method !== 'GET' && method !== 'HEAD') {
    if (csrf) headers['X-CSRF-Token'] = csrf;
    if (body !== undefined) {
      headers['Content-Type'] = 'application/json';
      init.body = JSON.stringify(body);
    }
  }
  let res: Response;
  try {
    res = await fetch(url, init);
  } catch {
    throw new ApiError(0, OFFLINE);
  }
  const isJSON = (res.headers.get('Content-Type') || '').includes('application/json');
  let data: unknown = null;
  if (isJSON) {
    try {
      data = await res.json();
    } catch {
      data = null;
    }
  }
  if (!res.ok) {
    if (res.status === 401 && url.startsWith('/api/admin/') && !url.startsWith('/api/admin/login') && onUnauthorized) {
      onUnauthorized();
    }
    const e = data as { error?: string; field?: string } | null;
    if (e && typeof e.error === 'string') throw new ApiError(res.status, e.error, e.field || undefined);
    throw new ApiError(res.status, OFFLINE);
  }
  if (!isJSON) throw new ApiError(res.status, OFFLINE);
  return data as T;
}

export function get<T>(path: string, params?: Params): Promise<T> {
  return request<T>('GET', withQuery(path, params));
}

export function send<T>(method: 'POST' | 'PUT' | 'DELETE', path: string, body?: unknown): Promise<T> {
  return request<T>(method, path, body);
}

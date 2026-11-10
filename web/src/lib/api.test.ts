import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError, get, send, setCsrf } from './api';

function respond(status: number, body: string, type = 'application/json') {
  return vi.fn(async () => new Response(body, { status, headers: { 'Content-Type': type } }));
}

const fail = (p: Promise<unknown>): Promise<ApiError> =>
  p.then(
    () => { throw new Error('expected rejection'); },
    (e: ApiError) => e,
  );

afterEach(() => {
  vi.unstubAllGlobals();
  setCsrf('');
});

describe('api', () => {
  it('maps error bodies to ApiError', async () => {
    vi.stubGlobal('fetch', respond(400, '{"error":"Mesaj","field":"name"}'));
    const err = await fail(get('/api/x'));
    expect(err).toBeInstanceOf(ApiError);
    expect(err.status).toBe(400);
    expect(err.field).toBe('name');
    expect(err.message).toBe('Mesaj');
  });
  it('sends the CSRF token on mutations', async () => {
    const f = respond(200, '{"ok":true}');
    vi.stubGlobal('fetch', f);
    setCsrf('t');
    await send('PUT', '/api/admin/settings', { a: 1 });
    const init = (f.mock.calls[0] as unknown[])[1] as RequestInit;
    expect((init.headers as Record<string, string>)['X-CSRF-Token']).toBe('t');
    expect(init.credentials).toBe('same-origin');
    expect(init.body).toBe('{"a":1}');
  });
  it('network failure', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => { throw new TypeError('Failed to fetch'); }));
    const err = await fail(get('/api/x'));
    expect(err.status).toBe(0);
    expect(err.message).toBe('Serverul nu răspunde.');
  });
  it('non-JSON error response', async () => {
    vi.stubGlobal('fetch', respond(502, '<html>bad gateway</html>', 'text/html'));
    const err = await fail(get('/api/x'));
    expect(err.status).toBe(502);
    expect(err.message).toBe('Serverul nu răspunde.');
  });
  it('adds query parameters', async () => {
    const f = respond(200, '{}');
    vi.stubGlobal('fetch', f);
    await get('/api/stats', { from: '1', location: '3' });
    expect((f.mock.calls[0] as unknown[])[0]).toBe('/api/stats?from=1&location=3');
  });
});

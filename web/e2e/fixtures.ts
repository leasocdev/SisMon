import { test as base, expect, type APIRequestContext, type Page, request as pwRequest } from '@playwright/test';

export const BASE = 'http://127.0.0.1:18501';
export const CONTROL = 'http://127.0.0.1:18502';
export const PASSWORD = 'parola-e2e-12345';

export async function control(path: string, body: unknown = {}, ctrl = CONTROL): Promise<void> {
  const res = await fetch(ctrl + path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
  if (!res.ok) throw new Error(`control ${path}: ${res.status}`);
}

export function setHost(host: string, result: 'ok' | 'timeout' | 'permission', latency_ms = 5): Promise<void> {
  return control('/host', { host, result, latency_ms });
}

export interface AdminApi {
  ctx: APIRequestContext;
  csrf: string;
  get<T = unknown>(path: string): Promise<T>;
  send<T = unknown>(method: 'POST' | 'PUT' | 'DELETE', path: string, body?: unknown): Promise<T>;
  dispose(): Promise<void>;
}

/** Logs in through the API (own cookie jar) to create test data. */
export async function adminApi(base = BASE, password = PASSWORD): Promise<AdminApi> {
  const ctx = await pwRequest.newContext({ baseURL: base, extraHTTPHeaders: { Origin: base } });
  const res = await ctx.post('/api/admin/login', { data: { password } });
  if (!res.ok()) throw new Error(`login: ${res.status()} ${await res.text()}`);
  const { csrf } = (await res.json()) as { csrf: string };
  const api: AdminApi = {
    ctx,
    csrf,
    async get<T>(path: string) {
      const r = await ctx.get(path);
      if (!r.ok()) throw new Error(`${path}: ${r.status()} ${await r.text()}`);
      return (await r.json()) as T;
    },
    async send<T>(method: 'POST' | 'PUT' | 'DELETE', path: string, body?: unknown) {
      const r = await ctx.fetch(path, { method, data: body, headers: { 'X-CSRF-Token': csrf } });
      if (!r.ok()) throw new Error(`${method} ${path}: ${r.status()} ${await r.text()}`);
      return (await r.json()) as T;
    },
    dispose: () => ctx.dispose(),
  };
  return api;
}

export interface TestLocation {
  name: string;
  lat?: number | null;
  lon?: number | null;
  devices: { name: string; host: string; is_primary?: boolean; kind?: string }[];
}

/** Creates a location through the admin API and returns its id. */
export async function createLocation(api: AdminApi, loc: TestLocation): Promise<number> {
  const body = {
    name: loc.name,
    address: '',
    notes: '',
    lat: loc.lat ?? null,
    lon: loc.lon ?? null,
    devices: loc.devices.map((d, i) => ({ name: d.name, host: d.host, kind: d.kind ?? '', is_primary: d.is_primary ?? i === 0, overrides: {} })),
  };
  const r = await api.send<{ id: number }>('POST', '/api/admin/locations', body);
  return r.id;
}

export async function locationId(name: string, base = BASE): Promise<number> {
  const r = await fetch(base + '/api/state');
  const s = (await r.json()) as { locations: { id: number; name: string }[] };
  const l = s.locations.find((x) => x.name === name);
  if (!l) throw new Error('no location ' + name);
  return l.id;
}

type Fixtures = {
  /** Console messages matching one of these are tolerated in this test. */
  allowConsole: RegExp[];
  page: Page;
};

export const test = base.extend<Fixtures>({
  allowConsole: [[], { option: true }],
  page: async ({ page, allowConsole }, use) => {
    await control('/reset');
    const problems: string[] = [];
    await page.addInitScript(() => {
      document.addEventListener('securitypolicyviolation', (e) => {
        (window as unknown as { __csp: string[] }).__csp ??= [];
        (window as unknown as { __csp: string[] }).__csp.push(`${e.violatedDirective} ${e.blockedURI}`);
      });
    });
    page.on('pageerror', (err) => problems.push('pageerror: ' + err.message));
    page.on('console', (msg) => {
      if (msg.type() !== 'error') return;
      const text = msg.text();
      if (/^Failed to load resource: the server responded with a status of/.test(text)) return;
      if (allowConsole.some((re) => re.test(text))) return;
      problems.push('console: ' + text);
    });
    await use(page);
    const csp = await page.evaluate(() => (window as unknown as { __csp?: string[] }).__csp ?? []).catch(() => []);
    expect([...problems, ...csp.map((c) => 'CSP: ' + c)], 'erori de consolă sau încălcări CSP').toEqual([]);
  },
});

export { expect };

/** Waits until the card of a location shows the given state text. */
export async function waitState(page: Page, name: string, text: string): Promise<void> {
  await expect(page.locator('.loc-card', { hasText: name }).first()).toContainText(text, { timeout: 20_000 });
}

/** Logs in through the UI. */
export async function login(page: Page, password = PASSWORD, base = ''): Promise<void> {
  await page.goto(base + '/administrare');
  await page.getByLabel('Parola', { exact: true }).fill(password);
  await page.getByRole('button', { name: 'Autentificare' }).click();
  await expect(page.getByRole('navigation', { name: 'Administrare' })).toBeVisible();
}

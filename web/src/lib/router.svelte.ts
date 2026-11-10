// Minimal History-API router.

export type RouteName =
  | 'monitor' | 'location' | 'dispatch' | 'map' | 'stats' | 'report'
  | 'admin' | 'adminLocation' | 'adminPage' | 'notFound';

export interface Route {
  name: RouteName;
  params: Record<string, string>;
  query: Record<string, string>;
  path: string;
}

export const ADMIN_PAGES = ['setari', 'identitate', 'backup', 'audit', 'diagnostic', 'parola', 'despre'] as const;

const patterns: [RouteName, RegExp, string[]][] = [
  ['monitor', /^\/$/, []],
  ['location', /^\/locatie\/(\d+)$/, ['id']],
  ['dispatch', /^\/dispecerat$/, []],
  ['map', /^\/harta$/, []],
  ['stats', /^\/statistici$/, []],
  ['report', /^\/raport-24h$/, []],
  ['admin', /^\/administrare$/, []],
  ['adminLocation', /^\/administrare\/locatie\/(\d+|nou)$/, ['id']],
  ['adminPage', /^\/administrare\/([a-z]+)$/, ['page']],
];

export function match(url: string): Route {
  const qi = url.indexOf('?');
  const rawPath = qi >= 0 ? url.slice(0, qi) : url;
  const rawQuery = qi >= 0 ? url.slice(qi + 1) : '';
  const path = rawPath.length > 1 ? rawPath.replace(/\/+$/, '') : rawPath;
  const query = Object.fromEntries(new URLSearchParams(rawQuery));
  for (const [name, re, keys] of patterns) {
    const m = re.exec(path);
    if (!m) continue;
    const params: Record<string, string> = {};
    keys.forEach((k, i) => (params[k] = decodeURIComponent(m[i + 1])));
    if (name === 'adminPage' && !(ADMIN_PAGES as readonly string[]).includes(params.page)) break;
    return { name, params, query, path };
  }
  return { name: 'notFound', params: {}, query, path };
}

function current(): string {
  return typeof location === 'undefined' ? '/' : location.pathname + location.search;
}

export const router = $state<{ route: Route }>({ route: match(current()) });

type Guard = () => boolean | Promise<boolean>;
const guards = new Set<Guard>();

/** Registers a guard asked before leaving the current page; returns the unregister function. */
export function beforeLeave(fn: Guard): () => void {
  guards.add(fn);
  return () => {
    guards.delete(fn);
  };
}

async function allowed(): Promise<boolean> {
  for (const g of [...guards]) {
    if (!(await g())) return false;
  }
  return true;
}

export function toSearch(q: Record<string, string>): string {
  const s = new URLSearchParams(q).toString();
  return s ? '?' + s : '';
}

export async function navigate(url: string, opts: { replace?: boolean; force?: boolean } = {}): Promise<boolean> {
  if (url === current()) return true;
  const next = match(url);
  const samePage = next.path === router.route.path;
  if (!opts.force && !samePage && !(await allowed())) return false;
  const idx = historyIndex();
  if (opts.replace) history.replaceState({ nwIdx: idx }, '', url);
  else history.pushState({ nwIdx: idx + 1 }, '', url);
  router.route = next;
  return true;
}

/** Position of the current entry in this app's history (0 = the page the tab opened on). */
function historyIndex(): number {
  const s = history.state as { nwIdx?: number } | null;
  return typeof s?.nwIdx === 'number' ? s.nwIdx : 0;
}

/** True when the previous history entry belongs to this app, so history.back() stays in it. */
export function canGoBack(): boolean {
  return historyIndex() > 0;
}

/** Installs link interception and back/forward handling (call once). */
export function startRouter(): void {
  if (typeof history.state?.nwIdx !== 'number') history.replaceState({ nwIdx: 0 }, '');
  document.addEventListener('click', (ev) => {
    if (ev.defaultPrevented || ev.button !== 0 || ev.metaKey || ev.ctrlKey || ev.shiftKey || ev.altKey) return;
    const a = (ev.target as Element | null)?.closest('a');
    if (!a || a.target || a.hasAttribute('download') || a.origin !== location.origin) return;
    const href = a.pathname + a.search;
    if (href.startsWith('/api/') || href.startsWith('/tiles/')) return;
    ev.preventDefault();
    navigate(href);
  });
  let restoring = false;
  window.addEventListener('popstate', async () => {
    if (restoring) {
      restoring = false;
      return;
    }
    const next = match(current());
    if (next.path !== router.route.path && !(await allowed())) {
      restoring = true;
      history.pushState({ nwIdx: historyIndex() + 1 }, '', router.route.path + toSearch(router.route.query));
      return;
    }
    router.route = next;
  });
}

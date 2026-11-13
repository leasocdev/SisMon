<script lang="ts">
  import { onMount, tick } from 'svelte';
  import IncidentList from '../components/IncidentList.svelte';
  import LatencyChart from '../components/LatencyChart.svelte';
  import MiniTimeline from '../components/MiniTimeline.svelte';
  import OutageList from '../components/OutageList.svelte';
  import StatusBadge from '../components/StatusBadge.svelte';
  import { get } from '../lib/api';
  import { live } from '../lib/live.svelte';
  import { canGoBack, navigate } from '../lib/router.svelte';
  import { session } from '../lib/session.svelte';
  import { errorText, fmtLatency, fmtLoss, fmtPct, pendingText, STATE_SYMBOL } from '../lib/status';
  import { fmtDateTime, fmtDuration, fmtSince } from '../lib/time';
  import type { IncidentRow, LocationRow, OutageRow, Point, Report, Segment } from '../lib/types';

  let { id }: { id: number } = $props();

  type Period = '24h' | '7z' | '30z';
  const PERIODS: [Period, string, number][] = [
    ['24h', '24 h', 1],
    ['7z', '7 z', 7],
    ['30z', '30 z', 30],
  ];

  let period = $state<Period>('24h');
  let range = $state({ from: 0, to: 0 });
  let locTimeline = $state<Segment[]>([]);
  let devTimelines = $state<Record<number, Segment[]>>({});
  let incidents = $state<IncidentRow[]>([]);
  let outages = $state<OutageRow[]>([]);
  let summary = $state<LocationRow | null>(null);
  let latency = $state<Point[]>([]);
  let selected = $state<number | null>(null);
  let error = $state('');
  let panel: HTMLElement;

  const loc = $derived(live.location(id));
  const now = $derived((live.tick, live.serverNow()));
  const notFound = $derived(live.loaded && !loc);

  let loadSeq = 0;
  async function load() {
    const my = ++loadSeq;
    const days = PERIODS.find((p) => p[0] === period)![2];
    const to = Math.round(live.serverNow());
    const from = to - days * 86_400_000;
    const q = { from, to };
    const devs = live.location(id)?.devices ?? [];
    // Each part is shown as soon as it loads; one failed request does not hide the others.
    const [lt, rep, ...dts] = await Promise.allSettled([
      get<{ segments: Segment[] }>(`/api/locations/${id}/timeline`, q),
      get<Report>('/api/stats', { ...q, location: id }),
      ...devs.map((d) => get<{ segments: Segment[] }>(`/api/devices/${d.id}/timeline`, q)),
    ]);
    if (my !== loadSeq) return; // a newer period was chosen meanwhile
    const failed: string[] = [];
    range = q;
    if (lt.status === 'fulfilled') locTimeline = (lt.value as { segments: Segment[] }).segments;
    else failed.push('cronologia locației');
    if (rep.status === 'fulfilled') {
      const r = rep.value as Report;
      incidents = r.incidents;
      outages = r.outages;
      summary = r.locations.find((x) => x.id === id) ?? null;
    } else failed.push('statisticile');
    const map: Record<number, Segment[]> = {};
    devs.forEach((d, i) => {
      const res = dts[i];
      if (res.status === 'fulfilled') map[d.id] = (res.value as { segments: Segment[] }).segments;
      else failed.push(`cronologia echipamentului ${d.name}`);
    });
    devTimelines = map;
    const first = [lt, rep, ...dts].find((r) => r.status === 'rejected') as PromiseRejectedResult | undefined;
    error = failed.length ? `Nu s-au putut încărca: ${failed.join(', ')} (${(first?.reason as Error)?.message ?? 'eroare'}).` : '';
    await loadLatency();
  }

  let latencyError = $state('');
  async function loadLatency() {
    if (selected == null) return;
    try {
      latency = (await get<{ points: Point[] }>(`/api/devices/${selected}/latency`, range)).points;
      latencyError = '';
    } catch (e) {
      latencyError = (e as Error).message;
    }
  }

  function setPeriod(p: Period) {
    period = p;
    load();
  }

  function select(did: number) {
    selected = did;
    loadLatency();
  }

  async function close() {
    const focusCard = () => (document.querySelector(`.loc-card[data-id="${id}"]`) as HTMLElement | null)?.focus();
    if (canGoBack()) {
      // back to where the panel was opened from (e.g. the dispatch screen with its options)
      window.addEventListener('popstate', () => setTimeout(focusCard, 0), { once: true });
      history.back();
      return;
    }
    if (await navigate('/')) {
      await tick();
      (document.querySelector(`.loc-card[data-id="${id}"]`) as HTMLElement | null)?.focus();
    }
  }

  let started = false;
  $effect(() => {
    if (started || !live.loaded) return;
    started = true;
    const l = live.location(id);
    selected = l?.devices.find((d) => d.is_primary)?.id ?? l?.devices[0]?.id ?? null;
    load();
  });

  // Keeps keyboard focus inside the modal panel.
  function trapFocus(e: KeyboardEvent) {
    const items = [...panel.querySelectorAll<HTMLElement>('a[href], button:not([disabled]), input, select, textarea, [tabindex]:not([tabindex="-1"])')].filter(
      (el) => el.offsetParent !== null,
    );
    if (!items.length) return;
    const first = items[0];
    const last = items[items.length - 1];
    const active = document.activeElement as HTMLElement | null;
    if (!panel.contains(active)) {
      e.preventDefault();
      first.focus();
    } else if (e.shiftKey && (active === first || active === panel)) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && active === last) {
      e.preventDefault();
      first.focus();
    }
  }

  onMount(() => {
    panel.focus();
    // "Editează" is offered to a signed-in administrator only
    if (!session.info) session.refresh().catch(() => {});
    // the timelines and the chart move on with time, even without state changes
    const every = setInterval(() => live.loaded && load(), 60_000);
    let t: ReturnType<typeof setTimeout> | null = null;
    const off = live.onChange((_p, n) => {
      if (n?.id !== id) return;
      if (t) clearTimeout(t);
      t = setTimeout(load, 5000);
    });
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        close();
      } else if (e.key === 'Tab') {
        trapFocus(e);
      }
    };
    document.addEventListener('keydown', onKey);
    return () => {
      off();
      clearInterval(every);
      if (t) clearTimeout(t);
      document.removeEventListener('keydown', onKey);
    };
  });
</script>

<div class="backdrop" onclick={close} aria-hidden="true"></div>
<div class="panel" role="dialog" aria-modal="true" aria-labelledby="panel-title" tabindex="-1" bind:this={panel}>
  <header class="phead">
    <h2 id="panel-title">{loc?.name ?? (notFound ? 'Locație' : '…')}</h2>
    <button type="button" onclick={close}>✕ Închide</button>
  </header>

  {#if notFound}
    <p class="notice">Locația nu există sau a fost arhivată.</p>
  {:else if loc}
    <div class="summary">
      <span class="panel-state"><StatusBadge state={loc.state} stale={live.stale} size="large" /></span>
      {#if loc.since > 0}<span class="muted">{fmtSince(loc.since, now)}</span>{/if}
      {#if loc.incomplete}<span class="muted">· date incomplete</span>{/if}
    </div>
    {#if loc.address}<p class="addr">{loc.address}</p>{/if}
    {#if loc.notes}<p class="notes">{loc.notes}</p>{/if}

    <div class="bar">
      <div class="periods" role="group" aria-label="Perioadă">
        {#each PERIODS as [key, label] (key)}
          <button type="button" aria-pressed={period === key} class:on={period === key} onclick={() => setPeriod(key)}>{label}</button>
        {/each}
      </div>
      <nav class="links" aria-label="Legături locație">
        <a href="/statistici?location={id}&preset={period}">Statistici</a>
        {#if session.info?.authenticated}<a href="/administrare/locatie/{id}">Editează</a>{/if}
      </nav>
    </div>
    {#if summary}
      <p class="psum">
        Disponibilitate <strong>{fmtPct(summary.availability)}</strong>
        · <strong>{summary.outages}</strong> {summary.outages === 1 ? 'cădere' : 'căderi'}
        · {summary.durations.down > 0 ? `${fmtDuration(summary.durations.down)} indisponibilă` : 'fără timp de indisponibilitate'}
      </p>
    {/if}
    {#if error}<p class="notice danger" role="alert">{error}</p>{/if}

    <h3>Cronologie locație</h3>
    <MiniTimeline segments={locTimeline} from={range.from} to={range.to} tall label="Cronologia locației" />
    <div class="axis muted"><span>{range.from ? fmtDateTime(range.from) : ''}</span><span>{range.to ? fmtDateTime(range.to) : ''}</span></div>

    <h3>Echipamente</h3>
    <ul class="devs">
      {#each loc.devices as d (d.id)}
        <li class:sel={selected === d.id}>
          <div class="drow">
            <span class="dev-sym t-{d.state}" aria-hidden="true">{STATE_SYMBOL[d.state]}</span>
            <span class="dn">{#if d.is_primary}<span class="star" title="Echipament primar">★</span>{/if}{d.name}</span>
            <span class="host">{d.host}</span>
            <StatusBadge state={d.state} size="small" />
            {#if d.since > 0}<span class="dsince muted">{fmtSince(d.since, now)}</span>{/if}
            <button type="button" class="lat" aria-pressed={selected === d.id} onclick={() => select(d.id)}>Latență</button>
          </div>
          <div class="ddet muted">
            {#if pendingText(d)}<span class="pending">{pendingText(d)}</span>{/if}
            {#if d.error_code}<span>{errorText(d.error_code)}</span>{/if}
            {#if d.dns_warning}<span class="t-degraded">⚠ DNS nerezolvat — se folosește ultima adresă cunoscută</span>{/if}
            <span>Latență {fmtLatency(d.latency_ms)}</span>
            <span>Pierderi {fmtLoss(d.loss_pct)}</span>
            <span>Ultima verificare: {d.last_check ? fmtDateTime(d.last_check) : '—'}</span>
          </div>
          <MiniTimeline segments={devTimelines[d.id] ?? []} from={range.from} to={range.to} label="Cronologia echipamentului {d.name}" />
        </li>
      {/each}
    </ul>
    {#if loc.disabled_devices?.length}
      <div class="disabled-devs">
        <p>
          {loc.disabled_devices.length === 1 ? 'Echipament dezactivat temporar' : 'Echipamente dezactivate temporar'}
          — nu se monitorizează și nu influențează starea locației:
        </p>
        <ul>
          {#each loc.disabled_devices as d (d.id)}
            <li>{d.name} <span class="host">{d.host}</span> — {d.until ? `până la ${fmtDateTime(d.until)}` : 'până la reactivare manuală'}</li>
          {/each}
        </ul>
      </div>
    {/if}

    {#if loc.devices.length}
      <h3>Latență — {loc.devices.find((d) => d.id === selected)?.name ?? ''}</h3>
      {#if latencyError}<p class="notice danger">{latencyError}</p>{/if}
      <LatencyChart points={latency} from={range.from} to={range.to} />
    {/if}

    <h3>Perioade de indisponibilitate</h3>
    <OutageList {outages} showLocation={false} />

    <h3>Incidente</h3>
    <IncidentList {incidents} showLocation={false} />
  {:else}
    <p class="muted">Se încarcă…</p>
  {/if}
</div>

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    background: rgb(0 0 0 / 0.35);
    z-index: 900;
  }
  .panel {
    position: fixed;
    top: 0;
    right: 0;
    bottom: 0;
    width: min(760px, 100vw);
    background: var(--bg);
    border-left: 1px solid var(--border);
    z-index: 1000;
    overflow-y: auto;
    padding: 12px 18px 24px;
  }
  .panel:focus {
    outline: none;
  }
  .phead {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 12px;
  }
  .phead h2 {
    margin: 0;
    overflow-wrap: anywhere;
  }
  .summary {
    display: flex;
    gap: 10px;
    align-items: center;
    margin: 10px 0;
    flex-wrap: wrap;
  }
  .addr,
  .notes {
    margin: 4px 0;
    white-space: pre-line;
  }
  .bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px 16px;
    flex-wrap: wrap;
    margin: 12px 0 4px;
  }
  .periods {
    display: flex;
    gap: 6px;
  }
  .links {
    display: flex;
    gap: 14px;
  }
  .psum {
    margin: 6px 0 0;
  }
  .dsince {
    font-size: 0.85em;
  }
  .disabled-devs {
    margin-top: 10px;
    padding: 8px 10px;
    border: 1px dashed var(--muted);
    border-radius: var(--radius);
    font-size: 0.9em;
  }
  .disabled-devs p {
    margin: 0 0 4px;
  }
  .disabled-devs ul {
    margin: 0;
    padding-left: 18px;
  }
  .periods .on {
    border-color: var(--focus);
    box-shadow: inset 0 0 0 1px var(--focus);
    font-weight: 600;
  }
  h3 {
    margin: 18px 0 6px;
    font-size: 1em;
  }
  .axis {
    display: flex;
    justify-content: space-between;
    font-size: 0.8em;
  }
  .devs {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }
  .devs li {
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 8px;
    background: var(--surface);
  }
  .devs li.sel {
    border-color: var(--focus);
  }
  .drow {
    display: flex;
    gap: 8px;
    align-items: center;
    flex-wrap: wrap;
  }
  .dn {
    font-weight: 600;
  }
  .star {
    color: var(--c-degraded);
    margin-right: 3px;
  }
  .host {
    font-family: var(--mono);
    color: var(--muted);
  }
  .lat {
    margin-left: auto;
    font-size: 0.85em;
    padding: 2px 8px;
  }
  .ddet {
    display: flex;
    flex-wrap: wrap;
    gap: 4px 12px;
    font-size: 0.85em;
    margin: 4px 0 6px;
  }
  .pending {
    color: var(--c-degraded);
    font-style: italic;
  }
</style>

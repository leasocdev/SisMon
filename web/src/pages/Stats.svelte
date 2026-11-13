<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import IncidentList from '../components/IncidentList.svelte';
  import PeriodPicker from '../components/PeriodPicker.svelte';
  import SortableTable from '../components/SortableTable.svelte';
  import MiniTimeline from '../components/MiniTimeline.svelte';
  import { ApiError, get, withQuery } from '../lib/api';
  import { live } from '../lib/live.svelte';
  import { navigate, router, toSearch } from '../lib/router.svelte';
  import { fmtLatency, fmtLoss, fmtPct, STATE_LABEL } from '../lib/status';
  import { apiParams, filterFromQuery, filterToQuery, range, type StatsFilter } from '../lib/statsquery';
  import type { Column } from '../lib/table';
  import { fmtDateTime, fmtDuration, fmtShort, TZ } from '../lib/time';
  import type { CatalogLocation, DeviceRow, LocationRow, OutageRow, Report, State } from '../lib/types';

  const filter = $derived(filterFromQuery(router.route.query, 0));
  let catalog = $state<CatalogLocation[]>([]);
  let report = $state<Report | null>(null);
  let error = $state('');
  let errorField = $state('');
  let loading = $state(false);
  let shown = $state({ from: 0, to: 0 });

  // Archived locations and devices are offered only with "Include arhivate".
  const visible = $derived(
    filter.archived
      ? catalog
      : catalog.filter((l) => !l.archived).map((l) => ({ ...l, devices: l.devices.filter((d) => !d.archived) })),
  );
  const kinds = $derived([...new Set(visible.flatMap((l) => l.devices.map((d) => d.kind)))].sort((a, b) => a.localeCompare(b, 'ro')));
  const devices = $derived(
    visible.filter((l) => !filter.location || l.id === filter.location).flatMap((l) => l.devices.map((d) => ({ ...d, location: l.name }))),
  );

  function update(patch: Partial<StatsFilter>) {
    const next: StatsFilter = { ...filter, ...patch };
    if (next.preset !== 'custom') {
      delete next.from;
      delete next.to;
    }
    navigate('/statistici' + toSearch(filterToQuery(next)), { replace: true });
  }

  function isArchived(f: StatsFilter): boolean {
    const loc = catalog.find((l) => l.id === f.location);
    const dev = catalog.flatMap((l) => l.devices).find((d) => d.id === f.device);
    return !!loc?.archived || !!dev?.archived;
  }

  function setArchived(on: boolean) {
    if (on) update({ archived: true });
    // a hidden selection would leave an invisible filter behind
    else if (isArchived(filter)) update({ archived: undefined, location: undefined, device: undefined });
    else update({ archived: undefined });
  }

  // A link to an archived location or device turns the switch on by itself.
  $effect(() => {
    const f = filter;
    if (catalog.length && !f.archived && isArchived(f)) untrack(() => update({ archived: true }));
  });

  function idOrUndef(v: string): number | undefined {
    const n = Number(v);
    return n > 0 ? n : undefined;
  }

  let seq = 0;
  async function load(f: StatsFilter) {
    const my = ++seq;
    loading = true;
    const now = live.serverNow();
    const params = apiParams(f, now);
    lastParams = params;
    try {
      const r = await get<Report>('/api/stats', params);
      if (my !== seq) return;
      report = r;
      shown = range(f, now);
      error = errorField = '';
    } catch (e) {
      if (my !== seq) return;
      error = (e as Error).message;
      errorField = e instanceof ApiError ? (e.field ?? '') : '';
    } finally {
      if (my === seq) loading = false;
    }
  }

  $effect(() => {
    const f = filter;
    untrack(() => load(f));
  });

  onMount(async () => {
    try {
      catalog = (await get<{ locations: CatalogLocation[] }>('/api/catalog')).locations;
    } catch (e) {
      error = (e as Error).message;
    }
  });

  // The exported files use exactly the parameters of the report on screen.
  let lastParams = $state<Record<string, string>>({});
  function exportHref(fmt: string): string {
    return withQuery(`/api/export/${fmt}`, lastParams);
  }

  const AVAIL_LOC =
    'Calculată doar din timpul cu date: (funcțională + degradată) față de (funcțională + degradată + indisponibilă). „Fără date” = monitorul nu a putut verifica și nu intră în calcul.';
  const AVAIL_DEV = 'Calculată doar din timpul cu date: funcțional față de (funcțional + indisponibil). Timpul fără date nu intră în calcul.';
  const COVERAGE = 'Partea din perioadă pentru care există date de monitorizare.';

  // the timeline header names the period it spans
  const locColumns: Column<LocationRow>[] = $derived([
    { key: 'name', label: 'Locație', value: (r) => r.name },
    { key: 'availability', label: 'Disponibilitate', value: (r) => r.availability, numeric: true, title: AVAIL_LOC },
    { key: 'down', label: 'Timp indisponibil', value: (r) => r.durations.down, numeric: true, title: 'Timpul total în care locația a fost indisponibilă în perioada selectată.' },
    { key: 'outages', label: 'Căderi', value: (r) => r.outages, numeric: true, title: 'Numărul perioadelor de indisponibilitate din perioada selectată.' },
    { key: 'coverage', label: 'Acoperire', value: (r) => r.coverage, numeric: true, title: COVERAGE },
    {
      key: 'timeline',
      label: report ? `Cronologie ${fmtShort(report.from)} – ${fmtShort(report.to)}` : 'Cronologie',
      value: () => null,
      sortable: false,
      cls: 'barcell',
      title: 'Stările locației în ordinea în care au avut loc. Treceți cu mouse-ul peste o porțiune pentru ore și durată.',
    },
  ]);

  const outageColumns: Column<OutageRow>[] = [
    { key: 'location', label: 'Locație', value: (r) => r.location },
    { key: 'state', label: 'Stare', value: (r) => STATE_LABEL[r.state] },
    { key: 'start', label: 'Început', value: (r) => r.start },
    { key: 'end', label: 'Sfârșit', value: (r) => r.end ?? Number.MAX_SAFE_INTEGER },
    { key: 'duration', label: 'Durată', value: (r) => r.duration_s, numeric: true },
    { key: 'devices', label: 'Echipamente căzute', value: (r) => r.devices.map((d) => d.name).join(', '), cls: 'wrapcell', title: 'Echipamentele principale căzute (indisponibilă) sau cele secundare (degradată).' },
  ];

  const LEGEND: State[] = ['up', 'degraded', 'down', 'error', 'unknown'];

  const columns: Column<DeviceRow>[] = [
    { key: 'location', label: 'Locație', value: (r) => r.location },
    { key: 'name', label: 'Echipament', value: (r) => r.name },
    { key: 'host', label: 'IP / host', value: (r) => r.host, cls: 'mono' },
    { key: 'kind', label: 'Tip', value: (r) => r.kind },
    { key: 'primary', label: '★', value: (r) => (r.is_primary ? 1 : 0) },
    { key: 'availability', label: 'Disponibilitate', value: (r) => r.availability, numeric: true, title: AVAIL_DEV },
    { key: 'coverage', label: 'Acoperire', value: (r) => r.coverage, numeric: true, title: COVERAGE },
    { key: 'latency', label: 'Latență', value: (r) => r.latency_ms, numeric: true },
    { key: 'loss', label: 'Pierderi', value: (r) => r.loss_pct, numeric: true },
    { key: 'incidents', label: 'Incidente', value: (r) => r.incidents, numeric: true, cls: 'incidents' },
    { key: 'mttr', label: 'MTTR', value: (r) => r.mttr_s, numeric: true },
  ];
</script>

<div class="page stats">
  <div class="titlebar">
    <h1>Statistici</h1>
    <div class="exports" role="group" aria-label="Export">
      <span class="muted">Export:</span>
      {#each [['csv', 'CSV'], ['xlsx', 'Excel'], ['pdf', 'PDF']] as [fmt, label] (fmt)}
        <a class="btn" href={exportHref(fmt)} download>{label}</a>
      {/each}
    </div>
  </div>
  <p class="period muted">
    Perioada: {shown.from ? fmtDateTime(shown.from) : '…'} – {shown.to ? fmtDateTime(shown.to) : '…'} · Ora: {TZ}
  </p>

  <section class="filters card">
    <PeriodPicker
      preset={filter.preset}
      from={range(filter, live.serverNow()).from}
      to={range(filter, live.serverNow()).to}
      onChange={(p) => update(p)}
    />
    <div class="selects">
      <label>
        Locație
        <select value={String(filter.location ?? '')} onchange={(e) => update({ location: idOrUndef(e.currentTarget.value), device: undefined })} aria-invalid={errorField === 'location'}>
          <option value="">Toate</option>
          {#each visible as l (l.id)}
            <option value={String(l.id)}>{l.name}{l.archived ? ' (arhivat)' : ''}</option>
          {/each}
        </select>
      </label>
      <label>
        Echipament
        <select value={String(filter.device ?? '')} onchange={(e) => update({ device: idOrUndef(e.currentTarget.value) })} aria-invalid={errorField === 'device'}>
          <option value="">Toate</option>
          {#each devices as d (d.id)}
            <option value={String(d.id)}>{d.location} · {d.name} ({d.host}){d.archived ? ' (arhivat)' : ''}</option>
          {/each}
        </select>
      </label>
      <label>
        Tip
        <select value={filter.kind ?? ''} onchange={(e) => update({ kind: e.currentTarget.value || undefined })}>
          <option value="">Toate</option>
          {#each kinds as k (k)}<option value={k}>{k}</option>{/each}
        </select>
      </label>
      <label>
        Incidente
        <select value={filter.incidents} onchange={(e) => update({ incidents: e.currentTarget.value as StatsFilter['incidents'] })}>
          <option value="all">Toate</option>
          <option value="active">În curs</option>
          <option value="closed">Încheiate</option>
        </select>
      </label>
      <label class="check" title="Include locațiile și echipamentele arhivate">
        <input type="checkbox" checked={!!filter.archived} onchange={(e) => setArchived(e.currentTarget.checked)} />
        Include arhivate
      </label>
    </div>
  </section>

  {#if error}<p class="notice danger" role="alert">{error}</p>{/if}
  {#if loading && !report}<p class="muted">Se încarcă…</p>{/if}

  {#if report}
    <div class="results" class:dim={loading} aria-busy={loading}>
    <h2>Locații</h2>
    <SortableTable rows={report.locations} columns={locColumns} rowKey={(r) => r.id} cls="locs" empty="Nu există locații în perioada selectată.">
      {#snippet cell(r, c)}
        {#if c.key === 'name'}{r.name}{r.archived ? ' (arhivat)' : ''}
        {:else if c.key === 'availability'}{fmtPct(r.availability)}
        {:else if c.key === 'down'}{r.durations.down > 0 ? fmtDuration(r.durations.down) : '—'}
        {:else if c.key === 'outages'}{r.outages}
        {:else if c.key === 'coverage'}{fmtPct(r.coverage)}
        {:else if c.key === 'timeline'}
          <MiniTimeline segments={r.segments} from={report!.from} to={report!.to} label="Cronologie {r.name}" tall />
        {/if}
      {/snippet}
    </SortableTable>
    <ul class="legend" aria-label="Legendă cronologie">
      {#each LEGEND as s (s)}
        <li><span class="sw f-{s}"></span>{STATE_LABEL[s]}</li>
      {/each}
    </ul>

    <div class="outages-head">
      <h2>Perioade de indisponibilitate</h2>
      <label class="check">
        <input type="checkbox" checked={!!filter.degraded} onchange={(e) => update({ degraded: e.currentTarget.checked || undefined })} />
        Include perioadele degradate
      </label>
    </div>
    {#if report.outages_total > report.outages.length}
      <p class="muted">Se afișează cele mai recente {report.outages.length} din {report.outages_total} perioade; lista completă se află în exportul CSV sau Excel.</p>
    {/if}
    <SortableTable rows={report.outages} columns={outageColumns} rowKey={(r) => `${r.location_id}-${r.start}`} cls="outages" empty="Nicio perioadă de indisponibilitate în perioada selectată.">
      {#snippet cell(r, c)}
        {#if c.key === 'state'}<span class="tag f-{r.state}">{STATE_LABEL[r.state]}</span>
        {:else if c.key === 'start'}{fmtDateTime(r.start)}
        {:else if c.key === 'end'}{r.end == null ? 'în curs' : fmtDateTime(r.end)}
        {:else if c.key === 'duration'}{fmtDuration(r.duration_s)}
        {:else if c.key === 'devices'}
          {#each r.devices as d, i (d.id)}{i > 0 ? ', ' : ''}<span title={d.host}>{d.name}</span>{:else}—{/each}
        {:else}{c.value(r) ?? '—'}{/if}
      {/snippet}
    </SortableTable>

    <h2>Echipamente</h2>
    <SortableTable rows={report.devices} {columns} rowKey={(r) => r.id} cls="devices">
      {#snippet cell(r, c)}
        {#if c.key === 'availability'}{fmtPct(r.availability)}
        {:else if c.key === 'coverage'}{fmtPct(r.coverage)}
        {:else if c.key === 'latency'}{fmtLatency(r.latency_ms)}
        {:else if c.key === 'loss'}{fmtLoss(r.loss_pct)}
        {:else if c.key === 'mttr'}{r.mttr_s == null ? '—' : fmtDuration(r.mttr_s)}
        {:else if c.key === 'primary'}{r.is_primary ? '★' : ''}
        {:else if c.key === 'name'}{r.name}{r.archived ? ' (arhivat)' : ''}
        {:else}{c.value(r) ?? '—'}{/if}
      {/snippet}
    </SortableTable>

    <h2>Incidente</h2>
    <IncidentList incidents={report.incidents} />
    </div>
  {/if}
</div>

<style>
  .titlebar {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 12px;
    flex-wrap: wrap;
  }
  .exports {
    display: flex;
    gap: 6px;
    align-items: center;
  }
  .btn {
    display: inline-block;
    padding: 4px 12px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface);
    color: var(--fg);
    text-decoration: none;
  }
  .period {
    margin-top: -6px;
  }
  .filters {
    padding: 10px;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }
  .selects {
    display: flex;
    flex-wrap: wrap;
    gap: 10px 16px;
  }
  .selects label {
    display: flex;
    flex-direction: column;
    gap: 2px;
    font-size: 0.9em;
    font-weight: 600;
  }
  .selects label.check {
    flex-direction: row;
    align-items: center;
    gap: 6px;
    align-self: flex-end;
    min-height: 2em;
  }
  .selects select {
    max-width: 320px;
    font-weight: 400;
  }
  /* Rezultatele vechi rămân vizibile, estompate, cât se încarcă cele noi. */
  .results.dim {
    opacity: 0.6;
  }
  h2 {
    font-size: 1.1em;
    margin: 20px 0 8px;
  }
  .stats :global(td.barcell) {
    width: 36%;
    min-width: 220px;
    white-space: normal;
  }
  .stats :global(td.wrapcell) {
    white-space: normal;
  }
  .stats :global(.tag) {
    display: inline-block;
    padding: 0 6px;
    border-radius: 3px;
    font-size: 0.85em;
  }
  .legend {
    list-style: none;
    display: flex;
    flex-wrap: wrap;
    gap: 4px 14px;
    margin: 6px 0 0;
    padding: 0;
    font-size: 0.85em;
  }
  .legend li {
    display: flex;
    align-items: center;
    gap: 5px;
  }
  .sw {
    display: inline-block;
    width: 12px;
    height: 12px;
    border-radius: 2px;
  }
  .sw.f-unknown {
    background: repeating-linear-gradient(135deg, var(--c-unknown) 0 3px, var(--surface) 3px 6px);
  }
  .outages-head {
    display: flex;
    align-items: baseline;
    gap: 16px;
    flex-wrap: wrap;
  }
  .outages-head .check {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 0.9em;
  }
  .stats :global(.mono) {
    font-family: var(--mono);
  }
</style>

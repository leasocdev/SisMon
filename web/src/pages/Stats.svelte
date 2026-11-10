<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import IncidentList from '../components/IncidentList.svelte';
  import PeriodPicker from '../components/PeriodPicker.svelte';
  import SortableTable from '../components/SortableTable.svelte';
  import StateBar from '../components/StateBar.svelte';
  import { ApiError, get } from '../lib/api';
  import { live } from '../lib/live.svelte';
  import { navigate, router, toSearch } from '../lib/router.svelte';
  import { fmtLatency, fmtLoss, fmtPct } from '../lib/status';
  import { apiParams, filterFromQuery, filterToQuery, range, type StatsFilter } from '../lib/statsquery';
  import type { Column } from '../lib/table';
  import { fmtDateTime, fmtDuration, TZ } from '../lib/time';
  import type { CatalogLocation, DeviceRow, Report } from '../lib/types';

  const filter = $derived(filterFromQuery(router.route.query, 0));
  let catalog = $state<CatalogLocation[]>([]);
  let report = $state<Report | null>(null);
  let error = $state('');
  let errorField = $state('');
  let loading = $state(false);
  let shown = $state({ from: 0, to: 0 });

  const kinds = $derived([...new Set(catalog.flatMap((l) => l.devices.map((d) => d.kind)))].sort((a, b) => a.localeCompare(b, 'ro')));
  const devices = $derived(
    catalog.filter((l) => !filter.location || l.id === filter.location).flatMap((l) => l.devices.map((d) => ({ ...d, location: l.name }))),
  );

  function update(patch: Partial<StatsFilter>) {
    const next: StatsFilter = { ...filter, ...patch };
    if (next.preset !== 'custom') {
      delete next.from;
      delete next.to;
    }
    navigate('/statistici' + toSearch(filterToQuery(next)), { replace: true });
  }

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

  const columns: Column<DeviceRow>[] = [
    { key: 'location', label: 'Locație', value: (r) => r.location },
    { key: 'name', label: 'Echipament', value: (r) => r.name },
    { key: 'host', label: 'IP / host', value: (r) => r.host, cls: 'mono' },
    { key: 'kind', label: 'Tip', value: (r) => r.kind },
    { key: 'primary', label: '★', value: (r) => (r.is_primary ? 1 : 0) },
    { key: 'availability', label: 'Disponibilitate', value: (r) => r.availability, numeric: true },
    { key: 'coverage', label: 'Acoperire', value: (r) => r.coverage, numeric: true },
    { key: 'latency', label: 'Latență', value: (r) => r.latency_ms, numeric: true },
    { key: 'loss', label: 'Pierderi', value: (r) => r.loss_pct, numeric: true },
    { key: 'incidents', label: 'Incidente', value: (r) => r.incidents, numeric: true, cls: 'incidents' },
    { key: 'mttr', label: 'MTTR', value: (r) => r.mttr_s, numeric: true },
  ];
</script>

<div class="page stats">
  <h1>Statistici</h1>
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
          {#each catalog as l (l.id)}
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
    </div>
  </section>

  {#if error}<p class="notice danger" role="alert">{error}</p>{/if}
  {#if loading && !report}<p class="muted">Se încarcă…</p>{/if}

  {#if report}
    <section class="kpis" aria-label="Indicatori" class:dim={loading}>
      <div class="kpi card kpi-availability"><span class="l">Disponibilitate</span><span class="v">{fmtPct(report.summary.availability)}</span></div>
      <div class="kpi card kpi-coverage"><span class="l">Acoperire</span><span class="v">{fmtPct(report.summary.coverage)}</span></div>
      <div class="kpi card kpi-incidents"><span class="l">Incidente</span><span class="v">{report.summary.incidents}</span></div>
      <div class="kpi card kpi-down"><span class="l">Timp indisponibil cumulat</span><span class="v">{fmtDuration(report.summary.down_s)}</span></div>
      <div class="kpi card kpi-mttr"><span class="l">MTTR</span><span class="v">{report.summary.mttr_s == null ? '—' : fmtDuration(report.summary.mttr_s)}</span></div>
    </section>

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

    <h2>Locații</h2>
    <table class="locs">
      <thead><tr><th scope="col">Locație</th><th scope="col">Disponibilitate</th><th scope="col">Acoperire</th><th scope="col">Stări în perioadă</th></tr></thead>
      <tbody>
        {#each report.locations as l (l.id)}
          <tr>
            <td>{l.name}{l.archived ? ' (arhivat)' : ''}</td>
            <td>{fmtPct(l.availability)}</td>
            <td>{fmtPct(l.coverage)}</td>
            <td class="barcell"><StateBar durations={l.durations} /></td>
          </tr>
        {:else}
          <tr><td colspan="4" class="muted">Nu există locații în perioada selectată.</td></tr>
        {/each}
      </tbody>
    </table>

    <h2>Incidente</h2>
    <IncidentList incidents={report.incidents} />
  {/if}
</div>

<style>
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
  .selects select {
    max-width: 320px;
    font-weight: 400;
  }
  .kpis {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(170px, 1fr));
    gap: 10px;
    margin: 14px 0;
  }
  .kpis.dim {
    opacity: 0.6;
  }
  .kpi {
    padding: 10px 12px;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .kpi .l {
    color: var(--muted);
    font-size: 0.88em;
  }
  .kpi .v {
    font-size: 1.6em;
    font-weight: 700;
  }
  h2 {
    font-size: 1.1em;
    margin: 20px 0 8px;
  }
  .barcell {
    width: 40%;
  }
  .stats :global(.mono) {
    font-family: var(--mono);
  }
</style>

<script lang="ts">
  import { onMount } from 'svelte';
  import IncidentList from '../components/IncidentList.svelte';
  import { get } from '../lib/api';
  import { live } from '../lib/live.svelte';
  import { errorText } from '../lib/status';
  import { fmtDateTime, fmtDuration, TZ } from '../lib/time';
  import type { Report24 } from '../lib/types';

  let report = $state<Report24 | null>(null);
  let error = $state('');

  const now = $derived((live.tick, live.serverNow()));

  async function load() {
    try {
      report = await get<Report24>('/api/report24');
      error = '';
    } catch (e) {
      error = (e as Error).message;
    }
  }

  onMount(() => {
    load();
    let t: ReturnType<typeof setTimeout> | null = null;
    const off = live.onChange(() => {
      if (t) clearTimeout(t);
      t = setTimeout(load, 3000);
    });
    const every = setInterval(load, 60_000);
    return () => {
      off();
      if (t) clearTimeout(t);
      clearInterval(every);
    };
  });
</script>

<div class="page report">
  <div class="title">
    {#if live.identity.logo_version > 0}<img class="rlogo" src="/branding/logo?v={live.identity.logo_version}" alt="" />{/if}
    <h1>{live.identity.report_title}</h1>
    <div class="actions no-print">
      <button type="button" onclick={() => window.print()}>Tipărește</button>
    </div>
  </div>
  <p class="muted">
    {live.identity.subtitle || live.identity.name} · Ultimele 24 de ore: {report ? `${fmtDateTime(report.from)} – ${fmtDateTime(report.to)}` : '…'} · Ora: {TZ}
  </p>
  {#if error}<p class="notice danger" role="alert">{error}</p>{/if}
  {#if live.stale}<p class="notice warn" role="alert">Date neactualizate: conexiunea cu serverul este întreruptă.</p>{/if}

  {#if report}
    <p class="summary">
      <span>Locații monitorizate: <strong>{report.locations}</strong></span>
      <span class="now" class:t-down={report.affected_now > 0} class:t-up={report.affected_now === 0}>
        Indisponibile acum: <strong>{report.affected_now}</strong>
      </span>
      <span>Întreruperi în 24 h: <strong>{report.incidents.length}</strong></span>
      <span>Incidente minore: <strong>{report.minor_incidents.length}</strong></span>
    </p>

    {#if report.current.length}
      <h2 class="current-h t-down">✕ Locații indisponibile acum ({report.current.length})</h2>
      <table class="current">
        <thead>
          <tr>
            <th scope="col">Locație</th><th scope="col">Echipament căzut</th>
            <th scope="col">Indisponibilă de la</th><th scope="col">Durată</th>
          </tr>
        </thead>
        <tbody>
          {#each report.current as c (c.location_id)}
            <tr>
              <td class="loc"><a href="/locatie/{c.location_id}">{c.location}</a></td>
              <td>
                <ul class="devs">
                  {#each c.devices as d (d.id)}
                    <li>{d.name} — <span class="cause">{errorText(d.error_code) || 'Indisponibil'}</span></li>
                  {/each}
                </ul>
              </td>
              <td>{fmtDateTime(c.since)}</td>
              <td class="dur">{fmtDuration((now - c.since) / 1000)}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    {:else}
      <h2 class="current-h t-up">✓ Toate locațiile sunt funcționale</h2>
    {/if}

    <h2>Întreruperi în ultimele 24 de ore</h2>
    <div class="outages major">
      <IncidentList incidents={report.incidents} empty="Nicio întrerupere în ultimele 24 de ore." />
    </div>

    <h2>Incidente minore în ultimele 24 de ore</h2>
    <div class="outages minor">
      <IncidentList incidents={report.minor_incidents} empty="Niciun incident minor în ultimele 24 de ore." />
    </div>
  {:else if !error}
    <p class="muted">Se încarcă…</p>
  {/if}
</div>

<style>
  .title {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 12px;
    flex-wrap: wrap;
  }
  .rlogo {
    height: 44px;
    max-width: 160px;
    object-fit: contain;
  }
  .title h1 {
    margin: 8px 0;
  }
  .actions {
    display: flex;
    gap: 8px;
    align-items: center;
  }
  .current-h {
    font-size: 1.15em;
  }
  .now {
    font-weight: 600;
  }
  table.current tbody tr {
    background: color-mix(in srgb, var(--c-down) 10%, var(--surface));
  }
  table.current tbody td:first-child {
    border-left: 5px solid var(--c-down);
  }
  .loc {
    font-weight: 700;
  }
  .devs {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  .cause {
    color: var(--c-down-text);
    font-weight: 600;
  }
  .dur {
    font-weight: 700;
  }
  @media print {
    /* Banda roșie rămâne vizibilă chiar dacă browserul nu tipărește fundalurile. */
    table.current tbody td:first-child {
      print-color-adjust: exact;
      -webkit-print-color-adjust: exact;
    }
  }
  .summary {
    display: flex;
    flex-wrap: wrap;
    gap: 8px 24px;
    font-size: 1.1em;
  }
  h2 {
    font-size: 1.1em;
    margin: 18px 0 6px;
  }
</style>

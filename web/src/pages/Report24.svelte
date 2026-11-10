<script lang="ts">
  import { onMount } from 'svelte';
  import IncidentList from '../components/IncidentList.svelte';
  import { get } from '../lib/api';
  import { live } from '../lib/live.svelte';
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
      <span>Afectate acum: <strong>{report.affected_now}</strong></span>
      <span>Întreruperi în 24 h: <strong>{report.incidents.length}</strong></span>
    </p>

    <h2>Situația curentă (echipamente primare indisponibile)</h2>
    <table class="current">
      <thead>
        <tr><th scope="col">Locație</th><th scope="col">Echipament primar</th><th scope="col">Indisponibil de la</th><th scope="col">Durată</th></tr>
      </thead>
      <tbody>
        {#each report.current as c (c.device_id)}
          <tr>
            <td><a href="/locatie/{c.location_id}">{c.location}</a></td>
            <td>{c.device}</td>
            <td>{fmtDateTime(c.since)}</td>
            <td>{fmtDuration((now - c.since) / 1000)}</td>
          </tr>
        {:else}
          <tr><td colspan="4" class="t-up">✓ Nicio locație afectată în acest moment.</td></tr>
        {/each}
      </tbody>
    </table>

    <h2>Întreruperi în ultimele 24 de ore</h2>
    <div class="outages">
      <IncidentList incidents={report.incidents} empty="Nicio întrerupere în ultimele 24 de ore." />
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

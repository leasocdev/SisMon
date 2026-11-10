<script lang="ts">
  import { onMount } from 'svelte';
  import { get } from '../../lib/api';
  import { fmtBytes, icmpMode } from '../../lib/format';
  import { healthWarning, live } from '../../lib/live.svelte';
  import { fmtDateTime } from '../../lib/time';
  import type { Health } from '../../lib/types';

  interface Diag {
    health: Health;
    version: string;
    data_dir: string;
    db_size_bytes: number;
    icmp_mode: string;
    now: number;
  }

  let d = $state<Diag | null>(null);
  let error = $state('');

  async function load() {
    try {
      d = await get<Diag>('/api/admin/diagnostic');
      error = '';
    } catch (e) {
      error = (e as Error).message;
    }
  }

  onMount(() => {
    load();
    const t = setInterval(load, 10_000);
    return () => clearInterval(t);
  });

  const warn = $derived(d ? healthWarning(d.health, live.intervalS, d.now) : '');
</script>

<h1>Diagnostic</h1>
{#if error}<p class="notice danger" role="alert">{error}</p>{/if}
{#if warn}<p class="notice warn" role="alert">⚠ {warn}</p>{/if}
{#if d}
  <dl class="diag card">
    <dt>Versiune</dt><dd>{d.version}</dd>
    <dt>Mod ICMP</dt><dd>{icmpMode(d.icmp_mode)}</dd>
    <dt>Ultimul ciclu</dt><dd>{d.health.last_cycle ? fmtDateTime(d.health.last_cycle) : '—'}</dd>
    <dt>Verificări întârziate</dt><dd>{d.health.late_checks}</dd>
    <dt>Durata medie a unei verificări</dt><dd>{d.health.avg_check_ms.toFixed(1).replace('.', ',')} ms</dd>
    <dt>Dimensiunea bazei</dt><dd>{fmtBytes(d.db_size_bytes)}</dd>
    <dt>Directorul de date</dt><dd class="mono">{d.data_dir}</dd>
    <dt>Ultimul backup</dt><dd>{d.health.last_backup ? fmtDateTime(d.health.last_backup) : '—'}{d.health.backup_error ? ' · ' + d.health.backup_error : ''}</dd>
    <dt>Interval de verificare</dt><dd>{live.intervalS} s</dd>
  </dl>
  <h2>Ultimele erori</h2>
  {#if d.health.errors?.length}
    <ul class="errs">
      {#each d.health.errors as e, i (i)}<li class="mono">{e}</li>{/each}
    </ul>
  {:else}
    <p class="muted">Nicio eroare.</p>
  {/if}
{/if}

<style>
  .diag {
    display: grid;
    grid-template-columns: max-content 1fr;
    gap: 6px 18px;
    padding: 12px 16px;
    margin: 0;
  }
  dt {
    color: var(--muted);
  }
  dd {
    margin: 0;
    overflow-wrap: anywhere;
  }
  .mono {
    font-family: var(--mono);
  }
  h2 {
    font-size: 1.05em;
    margin: 18px 0 6px;
  }
  .errs {
    margin: 0;
    padding-left: 18px;
    font-size: 0.9em;
  }
</style>

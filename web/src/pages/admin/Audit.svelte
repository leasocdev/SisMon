<script lang="ts">
  import { onMount } from 'svelte';
  import { get } from '../../lib/api';
  import { auditAction } from '../../lib/format';
  import { fmtDateTime } from '../../lib/time';
  import type { AuditEntry } from '../../lib/types';

  const PAGE = 50;
  let entries = $state<AuditEntry[]>([]);
  let more = $state(false);
  let error = $state('');
  let open = $state<Record<number, boolean>>({});

  async function load(before?: number) {
    try {
      const r = await get<{ entries: AuditEntry[] }>('/api/admin/audit', { limit: PAGE, before });
      entries = before ? [...entries, ...r.entries] : r.entries;
      more = r.entries.length === PAGE;
      error = '';
    } catch (e) {
      error = (e as Error).message;
    }
  }

  function pretty(s: string): string {
    try {
      return JSON.stringify(JSON.parse(s), null, 2);
    } catch {
      return s;
    }
  }

  onMount(() => load());
</script>

<h1>Jurnal audit</h1>
{#if error}<p class="notice danger" role="alert">{error}</p>{/if}
<table class="audit">
  <thead><tr><th scope="col">Data</th><th scope="col">Acțiune</th><th scope="col">Entitate</th><th scope="col">Detalii</th></tr></thead>
  <tbody>
    {#each entries as e (e.id)}
      <tr>
        <td class="nowrap">{fmtDateTime(e.ts)}</td>
        <td>{auditAction(e.action)}</td>
        <td>{e.entity}{e.entity_id ? ` #${e.entity_id}` : ''}</td>
        <td>
          <button type="button" class="link" aria-expanded={!!open[e.id]} onclick={() => (open[e.id] = !open[e.id])}>
            {open[e.id] ? 'Ascunde' : 'Arată'}
          </button>
          {#if open[e.id]}<pre>{pretty(e.details)}</pre>{/if}
        </td>
      </tr>
    {:else}
      <tr><td colspan="4" class="muted">Jurnalul este gol.</td></tr>
    {/each}
  </tbody>
</table>
{#if more}
  <p><button type="button" onclick={() => load(entries[entries.length - 1]?.id)}>Mai vechi</button></p>
{/if}

<style>
  .nowrap {
    white-space: nowrap;
  }
  pre {
    margin: 6px 0 0;
    font-family: var(--mono);
    font-size: 0.85em;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    max-width: 640px;
  }
  .link {
    border: none;
    background: none;
    padding: 0;
    color: var(--link);
    text-decoration: underline;
  }
</style>

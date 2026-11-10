<script lang="ts">
  import { fmtDateTime, fmtDuration } from '../lib/time';
  import type { IncidentRow } from '../lib/types';

  let { incidents, showLocation = true, empty = 'Niciun incident în perioada selectată.' }: { incidents: IncidentRow[]; showLocation?: boolean; empty?: string } =
    $props();
</script>

<div class="incidents">
  {#if incidents.length === 0}
    <p class="muted">{empty}</p>
  {:else}
    <table>
      <thead>
        <tr>
          {#if showLocation}<th scope="col">Locație</th>{/if}
          <th scope="col">Echipament</th>
          <th scope="col">Început</th>
          <th scope="col">Confirmat</th>
          <th scope="col">Sfârșit</th>
          <th scope="col">Durată</th>
          <th scope="col">Motiv</th>
        </tr>
      </thead>
      <tbody>
        {#each incidents as i (i.id)}
          <tr class:open={i.ended_at == null}>
            {#if showLocation}<td>{i.location}</td>{/if}
            <td>{i.device}</td>
            <td>{fmtDateTime(i.observed_at)}</td>
            <td>{fmtDateTime(i.confirmed_at)}</td>
            <td>{i.ended_at == null ? '—' : fmtDateTime(i.ended_at)}</td>
            <td>{fmtDuration(i.duration_s)}</td>
            <td>{i.reason_text}</td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}
</div>

<style>
  .incidents {
    overflow-x: auto;
  }
  tr.open td {
    color: var(--c-down);
    font-weight: 600;
  }
  td {
    white-space: nowrap;
  }
</style>

<script lang="ts">
  import { STATE_LABEL } from '../lib/status';
  import { fmtDateTime, fmtDuration } from '../lib/time';
  import type { OutageRow } from '../lib/types';

  let {
    outages,
    showLocation = true,
    empty = 'Nicio perioadă de indisponibilitate în perioada selectată.',
  }: { outages: OutageRow[]; showLocation?: boolean; empty?: string } = $props();
</script>

<div class="outages">
  {#if outages.length === 0}
    <p class="muted">{empty}</p>
  {:else}
    <table>
      <thead>
        <tr>
          {#if showLocation}<th scope="col">Locație</th>{/if}
          <th scope="col">Stare</th>
          <th scope="col">Început</th>
          <th scope="col">Sfârșit</th>
          <th scope="col">Durată</th>
          <th scope="col" title="Echipamentele principale căzute (indisponibilă) sau cele secundare (degradată).">Echipamente căzute</th>
        </tr>
      </thead>
      <tbody>
        {#each outages as o (`${o.location_id}-${o.start}`)}
          <tr class:open={o.end == null}>
            {#if showLocation}<td>{o.location}</td>{/if}
            <td><span class="tag f-{o.state}">{STATE_LABEL[o.state]}</span></td>
            <td>{fmtDateTime(o.start)}</td>
            <td>{o.end == null ? 'în curs' : fmtDateTime(o.end)}</td>
            <td>{fmtDuration(o.duration_s)}</td>
            <td class="devs">
              {#each o.devices as d, i (d.id)}{i > 0 ? ', ' : ''}<span title={d.host}>{d.name}</span>{:else}—{/each}
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}
</div>

<style>
  .outages {
    overflow-x: auto;
  }
  td {
    white-space: nowrap;
  }
  td.devs {
    white-space: normal;
  }
  tr.open td {
    font-weight: 600;
  }
  .tag {
    display: inline-block;
    padding: 0 6px;
    border-radius: 3px;
    font-size: 0.85em;
  }
</style>

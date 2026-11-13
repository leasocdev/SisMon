<script lang="ts" generics="T">
  import type { Snippet } from 'svelte';
  import type { Column } from '../lib/table';

  let {
    rows,
    columns,
    rowKey,
    cell,
    cls = '',
    empty = 'Nu există date.',
  }: {
    rows: T[];
    columns: Column<T>[];
    rowKey: (r: T) => string | number;
    cell?: Snippet<[T, Column<T>]>;
    cls?: string;
    empty?: string;
  } = $props();

  let sortKey = $state<string | null>(null);
  let dir = $state<'ascending' | 'descending'>('ascending');

  const collator = new Intl.Collator('ro', { numeric: true, sensitivity: 'base' });

  const sorted = $derived.by(() => {
    const col = columns.find((c) => c.key === sortKey);
    if (!col) return rows;
    const sign = dir === 'ascending' ? 1 : -1;
    return [...rows].sort((a, b) => {
      const va = col.value(a);
      const vb = col.value(b);
      if (va == null && vb == null) return 0;
      if (va == null) return 1; // empty values last
      if (vb == null) return -1;
      if (typeof va === 'number' && typeof vb === 'number') return (va - vb) * sign;
      return collator.compare(String(va), String(vb)) * sign;
    });
  });

  function sortBy(key: string) {
    if (sortKey === key) dir = dir === 'ascending' ? 'descending' : 'ascending';
    else {
      sortKey = key;
      dir = 'ascending';
    }
  }
</script>

<div class="wrap">
  <table class={cls}>
    <thead>
      <tr>
        {#each columns as c (c.key)}
          {#if c.sortable === false}
            <th scope="col" class:num={c.numeric} title={c.title}>{c.label}</th>
          {:else}
            <th scope="col" aria-sort={sortKey === c.key ? dir : 'none'} class:num={c.numeric} title={c.title}>
              <button type="button" class="sort" onclick={() => sortBy(c.key)}>
                {c.label}<span aria-hidden="true">{sortKey === c.key ? (dir === 'ascending' ? ' ▲' : ' ▼') : ''}</span>
              </button>
            </th>
          {/if}
        {/each}
      </tr>
    </thead>
    <tbody>
      {#each sorted as r (rowKey(r))}
        <tr>
          {#each columns as c (c.key)}
            <td class={c.cls} class:num={c.numeric}>
              {#if cell}{@render cell(r, c)}{:else}{c.value(r) ?? '—'}{/if}
            </td>
          {/each}
        </tr>
      {:else}
        <tr><td colspan={columns.length} class="muted">{empty}</td></tr>
      {/each}
    </tbody>
  </table>
</div>

<style>
  .wrap {
    overflow-x: auto;
  }
  .sort {
    border: none;
    background: none;
    padding: 0;
    font: inherit;
    font-weight: 600;
    color: inherit;
    cursor: pointer;
  }
  .num {
    text-align: right;
  }
  td {
    white-space: nowrap;
  }
</style>

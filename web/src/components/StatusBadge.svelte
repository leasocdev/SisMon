<script lang="ts">
  import { STATE_LABEL, STATE_SYMBOL } from '../lib/status';
  import type { State } from '../lib/types';

  let { state, stale = false, size = 'normal' }: { state: State; stale?: boolean; size?: 'small' | 'normal' | 'large' } = $props();
</script>

<span class="badge f-{stale ? 'unknown' : state} {size}">
  <span aria-hidden="true">{STATE_SYMBOL[state]}</span>
  {STATE_LABEL[state]}{#if stale}<span class="st"> · neactualizat</span>{/if}
</span>

<style>
  .badge {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    border-radius: 4px;
    padding: 2px 8px;
    font-weight: 600;
    font-size: 0.88em;
    white-space: nowrap;
  }
  .small {
    font-size: 0.78em;
    padding: 1px 6px;
  }
  .large {
    font-size: 1.15em;
    padding: 4px 12px;
  }
  .st {
    font-weight: 400;
  }
</style>

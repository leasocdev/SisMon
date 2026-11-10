<script lang="ts">
  import { onMount } from 'svelte';
  import { live } from '../lib/live.svelte';
  import { stateText } from '../lib/status';

  let messages = $state<{ id: number; text: string }[]>([]);
  let seq = 0;

  onMount(() =>
    live.onChange((prev, next) => {
      const msg = next ? `${next.name}: ${stateText(next.state)}` : prev ? `${prev.name}: eliminată din monitorizare` : '';
      if (msg) messages = [...messages, { id: seq++, text: msg }].slice(-3);
    }),
  );
</script>

<div class="sr-only" aria-live="polite" aria-atomic="false">
  {#each messages as m (m.id)}
    <p>{m.text}</p>
  {/each}
</div>

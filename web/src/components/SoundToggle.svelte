<script lang="ts">
  import { beeper, savePrefs, soundPrefs } from '../lib/sound.svelte';

  let { compact = false }: { compact?: boolean } = $props();

  async function toggle() {
    if (beeper.blocked) {
      await beeper.unlock();
      soundPrefs.on = true;
    } else {
      soundPrefs.on = !soundPrefs.on;
      if (soundPrefs.on) await beeper.unlock();
    }
    savePrefs({ ...soundPrefs });
  }

  function toggleDegraded(e: Event) {
    soundPrefs.degraded = (e.currentTarget as HTMLInputElement).checked;
    savePrefs({ ...soundPrefs });
  }
</script>

<span class="sound">
  <button type="button" onclick={toggle} aria-pressed={soundPrefs.on && !beeper.blocked} class:blocked={beeper.blocked}>
    {#if beeper.blocked && soundPrefs.on}
      🔇 Click pentru a activa sunetul
    {:else if soundPrefs.on}
      🔔 Sunet: pornit
    {:else}
      🔕 Sunet: oprit
    {/if}
  </button>
  {#if soundPrefs.on && !compact}
    <label class="deg"><input type="checkbox" checked={soundPrefs.degraded} onchange={toggleDegraded} /> și la degradare</label>
  {/if}
</span>

<style>
  .sound {
    display: inline-flex;
    align-items: center;
    gap: 8px;
  }
  .blocked {
    border-color: var(--c-degraded);
    font-weight: 600;
  }
  .deg {
    font-size: 0.88em;
    display: inline-flex;
    gap: 4px;
    align-items: center;
  }
</style>

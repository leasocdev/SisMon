<script lang="ts">
  import { tick } from 'svelte';
  import { confirmState } from '../lib/confirm.svelte';

  let cancelBtn = $state<HTMLButtonElement>();
  let typedInput = $state<HTMLInputElement>();
  let typedText = $state('');
  let previous: HTMLElement | null = null;

  $effect(() => {
    if (confirmState.current) {
      previous = document.activeElement as HTMLElement | null;
      typedText = '';
      tick().then(() => (typedInput ?? cancelBtn)?.focus());
    }
  });

  function done(ok: boolean) {
    confirmState.current?.resolve(ok);
    previous?.focus?.();
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault();
      done(false);
    }
    if (e.key === 'Tab') {
      // keep focus inside the dialog's controls
      const btns = [...(e.currentTarget as HTMLElement).querySelectorAll<HTMLElement>('input, button:not(:disabled)')];
      const i = btns.indexOf(document.activeElement as HTMLElement);
      e.preventDefault();
      btns[(i + (e.shiftKey ? -1 : 1) + btns.length) % btns.length]?.focus();
    }
  }
</script>

{#if confirmState.current}
  {@const c = confirmState.current}
  <div class="overlay">
    <div class="box card" role="alertdialog" aria-modal="true" aria-labelledby="confirm-msg" tabindex="-1" onkeydown={onKey}>
      <p id="confirm-msg">{c.message}</p>
      {#if c.typed !== undefined}
        <label class="typed">
          Pentru confirmare, tastați „{c.typed}”:
          <input type="text" bind:this={typedInput} bind:value={typedText} autocomplete="off" spellcheck="false"
            onkeydown={(e) => e.key === 'Enter' && typedText.trim() === c.typed && (e.preventDefault(), done(true))} />
        </label>
      {/if}
      <div class="btns">
        <button type="button" bind:this={cancelBtn} onclick={() => done(false)}>{c.cancel}</button>
        <button type="button" class="primary" class:danger={c.danger} disabled={c.typed !== undefined && typedText.trim() !== c.typed} onclick={() => done(true)}>{c.confirm}</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .overlay {
    position: fixed;
    inset: 0;
    z-index: 3000;
    background: rgb(0 0 0 / 0.45);
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 16px;
  }
  .box {
    max-width: 460px;
    padding: 18px 20px;
  }
  .typed {
    display: flex;
    flex-direction: column;
    gap: 4px;
    margin-bottom: 12px;
  }
  .btns {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
  }
  .danger {
    background: var(--c-down);
    border-color: var(--c-down);
    color: var(--on-down);
  }
</style>

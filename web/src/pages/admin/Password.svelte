<script lang="ts">
  import { ApiError, send } from '../../lib/api';
  import { session } from '../../lib/session.svelte';

  let old = $state('');
  let next = $state('');
  let repeat = $state('');
  let errors = $state<Record<string, string>>({});
  let error = $state('');
  let busy = $state(false);

  async function submit(e: Event) {
    e.preventDefault();
    errors = {};
    error = '';
    if (next !== repeat) {
      errors = { repeat: 'Parolele nu coincid.' };
      return;
    }
    const n = [...next].length;
    if (n < 12 || n > 200) {
      errors = { new: 'Parola trebuie să aibă între 12 și 200 de caractere.' };
      return;
    }
    busy = true;
    try {
      await send('PUT', '/api/admin/password', { old, new: next });
      session.loggedOut('Parola a fost schimbată. Autentificați-vă din nou.');
    } catch (err) {
      if (err instanceof ApiError && err.field) errors = { [err.field]: err.message };
      else error = (err as Error).message;
    } finally {
      busy = false;
    }
  }
</script>

  <h1>Parolă</h1>
  <form class="card pw" onsubmit={submit} novalidate>
  {#if error}<p class="notice danger" role="alert">{error}</p>{/if}
  <div class="field">
    <label for="pw-old">Parola actuală</label>
    <input id="pw-old" type="password" autocomplete="current-password" bind:value={old} aria-invalid={!!errors.old} />
    {#if errors.old}<p class="error-text">{errors.old}</p>{/if}
  </div>
  <div class="field">
    <label for="pw-new">Parola nouă</label>
    <input id="pw-new" type="password" autocomplete="new-password" bind:value={next} aria-invalid={!!errors.new} />
    {#if errors.new}<p class="error-text">{errors.new}</p>{/if}
  </div>
  <div class="field">
    <label for="pw-rep">Repetați parola nouă</label>
    <input id="pw-rep" type="password" autocomplete="new-password" bind:value={repeat} aria-invalid={!!errors.repeat} />
    {#if errors.repeat}<p class="error-text">{errors.repeat}</p>{/if}
  </div>
  <p class="muted">După schimbare, toate sesiunile deschise se închid.</p>
  <button type="submit" class="primary" disabled={busy}>Schimbă parola</button>
</form>

<style>
.pw {
  padding: 14px 16px;
  max-width: 480px;
}
  .pw input {
    width: 100%;
  }
</style>

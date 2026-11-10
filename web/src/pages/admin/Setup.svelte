<script lang="ts">
  import { session } from '../../lib/session.svelte';

  let password = $state('');
  let repeat = $state('');
  let error = $state('');
  let busy = $state(false);

  async function submit(e: Event) {
    e.preventDefault();
    error = '';
    const n = [...password].length;
    if (password !== repeat) {
      error = 'Parolele nu coincid.';
      return;
    }
    if (n < 12 || n > 200) {
      error = 'Parola trebuie să aibă între 12 și 200 de caractere.';
      return;
    }
    busy = true;
    try {
      await session.setup(password);
    } catch (err) {
      error = (err as Error).message;
    } finally {
      busy = false;
    }
  }
</script>

<form class="auth card" onsubmit={submit}>
  <h1>Configurare inițială</h1>
  <p>Nu există încă o parolă de administrator. Alegeți una (12–200 de caractere). Această pagină este disponibilă doar de pe server (localhost).</p>
  <div class="field">
    <label for="np">Parola nouă</label>
    <input id="np" type="password" autocomplete="new-password" bind:value={password} aria-invalid={!!error} />
  </div>
  <div class="field">
    <label for="rp">Repetați parola</label>
    <input id="rp" type="password" autocomplete="new-password" bind:value={repeat} aria-invalid={!!error} />
  </div>
  {#if error}<p class="error-text" role="alert">{error}</p>{/if}
  <button type="submit" class="primary" disabled={busy}>Setează parola</button>
</form>

<style>
  .auth {
    max-width: 480px;
    margin: 40px auto;
    padding: 20px 24px;
  }
  h1 {
    font-size: 1.3em;
    margin-top: 0;
  }
  input {
    width: 100%;
  }
</style>

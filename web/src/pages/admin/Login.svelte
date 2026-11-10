<script lang="ts">
  import { session } from '../../lib/session.svelte';

  let { notice = '' }: { notice?: string } = $props();

  let password = $state('');
  let error = $state('');
  let busy = $state(false);

  async function submit(e: Event) {
    e.preventDefault();
    busy = true;
    error = '';
    try {
      await session.login(password);
      password = '';
    } catch (err) {
      error = (err as Error).message;
    } finally {
      busy = false;
    }
  }
</script>

<form class="auth card" onsubmit={submit}>
  <h1>Autentificare administrator</h1>
  {#if session.expired}<p class="notice warn" role="status">Sesiunea a expirat. Autentificați-vă din nou.</p>{/if}
  {#if notice || session.notice}<p class="notice" role="status">{notice || session.notice}</p>{/if}
  {#if session.info && !session.info.tls}
    <p class="notice warn">Conexiune necriptată: parola se transmite în clar în rețea. Porniți serverul cu --tls auto.</p>
  {/if}
  <div class="field">
    <label for="pw">Parola</label>
    <!-- svelte-ignore a11y_autofocus -->
    <input id="pw" type="password" autocomplete="current-password" bind:value={password} required autofocus aria-invalid={!!error} />
  </div>
  {#if error}<p class="error-text" role="alert">{error}</p>{/if}
  <button type="submit" class="primary" disabled={busy || !password}>Autentificare</button>
</form>

<style>
  .auth {
    max-width: 440px;
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

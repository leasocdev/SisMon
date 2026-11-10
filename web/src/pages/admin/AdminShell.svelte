<script lang="ts">
  import { untrack, type Component } from 'svelte';
  import { router } from '../../lib/router.svelte';
  import { session } from '../../lib/session.svelte';
  import LoadFailed from '../../components/LoadFailed.svelte';
  import Login from './Login.svelte';
  import Setup from './Setup.svelte';

  const MENU = [
    { href: '/administrare', label: 'Locații', key: 'locatii' },
    { href: '/administrare/setari', label: 'Setări monitorizare', key: 'setari' },
    { href: '/administrare/identitate', label: 'Identitate', key: 'identitate' },
    { href: '/administrare/import', label: 'Import / export', key: 'import' },
    { href: '/administrare/backup', label: 'Backup-uri', key: 'backup' },
    { href: '/administrare/audit', label: 'Jurnal audit', key: 'audit' },
    { href: '/administrare/diagnostic', label: 'Diagnostic', key: 'diagnostic' },
    { href: '/administrare/parola', label: 'Parolă', key: 'parola' },
    { href: '/administrare/despre', label: 'Despre', key: 'despre' },
  ];

  type Loader = () => Promise<{ default: Component<Record<string, unknown>> }>;
  const PAGES: Record<string, Loader> = {
    locatii: () => import('./Locations.svelte') as unknown as ReturnType<Loader>,
    locatie: () => import('./LocationEditor.svelte') as unknown as ReturnType<Loader>,
    setari: () => import('./Settings.svelte') as unknown as ReturnType<Loader>,
    identitate: () => import('./Identity.svelte') as unknown as ReturnType<Loader>,
    import: () => import('./Import.svelte') as unknown as ReturnType<Loader>,
    backup: () => import('./Backups.svelte') as unknown as ReturnType<Loader>,
    audit: () => import('./Audit.svelte') as unknown as ReturnType<Loader>,
    diagnostic: () => import('./Diagnostic.svelte') as unknown as ReturnType<Loader>,
    parola: () => import('./Password.svelte') as unknown as ReturnType<Loader>,
    despre: () => import('./About.svelte') as unknown as ReturnType<Loader>,
  };

  const key = $derived(
    router.route.name === 'adminLocation' ? 'locatie' : router.route.name === 'adminPage' ? router.route.params.page : 'locatii',
  );
  const menuKey = $derived(key === 'locatie' ? 'locatii' : key);

  let loadError = $state('');

  // Re-check the session on every admin navigation, so an expired session is
  // noticed before the page acts on it.
  $effect(() => {
    void router.route.path;
    untrack(async () => {
      const was = session.info?.authenticated;
      try {
        await session.refresh();
        loadError = '';
        if (was && !session.info?.authenticated) session.expired = true;
      } catch (e) {
        loadError = (e as Error).message;
      }
    });
  });

  async function logout() {
    await session.logout();
  }
</script>

{#if loadError && !session.info}
  <div class="page"><p class="notice danger" role="alert">{loadError}</p></div>
{:else if !session.info}
  <div class="page"><p class="muted">Se încarcă…</p></div>
{:else if !session.info.configured}
  {#if session.info.setup_allowed}
    <Setup />
  {:else}
    <div class="page">
      <h1>Administrare</h1>
      <p class="notice warn">Parola nu este configurată. Rulați pe server: <code>netwatch set-password</code></p>
    </div>
  {/if}
{:else if !session.info.authenticated && !session.expired}
  <Login />
{:else}
  <div class="admin">
    <nav aria-label="Administrare" class="side">
      <ul>
        {#each MENU as m (m.key)}
          <li><a href={m.href} aria-current={menuKey === m.key ? 'page' : undefined}>{m.label}</a></li>
        {/each}
      </ul>
      <button type="button" class="logout" onclick={logout}>Ieșire</button>
    </nav>
    <section class="content">
      {#key router.route.path}
        {#await PAGES[key]()}
          <p class="muted">Se încarcă…</p>
        {:then m}
          <m.default />
        {:catch}
          <LoadFailed />
        {/await}
      {/key}
    </section>
  </div>
  {#if !session.info.authenticated}
    <!-- expired session: log in over the page so unsaved work stays -->
    <div class="login-overlay">
      <div role="dialog" aria-modal="true" aria-label="Autentificare administrator">
        <Login />
      </div>
    </div>
  {/if}
{/if}

<style>
  .admin {
    display: grid;
    grid-template-columns: 220px minmax(0, 1fr);
    gap: 16px;
    padding: 16px;
    max-width: 1600px;
    margin: 0 auto;
  }
  @media (max-width: 760px) {
    .admin {
      grid-template-columns: 1fr;
    }
  }
  .side ul {
    list-style: none;
    margin: 0 0 12px;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .side a {
    display: block;
    padding: 6px 10px;
    border-radius: var(--radius);
    color: var(--fg);
    text-decoration: none;
  }
  .side a:hover {
    background: var(--surface-2);
  }
  .side a[aria-current='page'] {
    background: var(--surface-2);
    font-weight: 600;
    box-shadow: inset 3px 0 0 var(--focus);
  }
  .content {
    min-width: 0;
  }
  .login-overlay {
    position: fixed;
    inset: 0;
    z-index: 2500;
    background: rgb(0 0 0 / 0.45);
    overflow-y: auto;
  }
</style>

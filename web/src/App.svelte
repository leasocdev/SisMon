<script lang="ts">
  import { onMount } from 'svelte';
  import Announcer from './components/Announcer.svelte';
  import ConfirmHost from './components/ConfirmHost.svelte';
  import ConnectionBadge from './components/ConnectionBadge.svelte';
  import Header from './components/Header.svelte';
  import DemoBanner from './components/DemoBanner.svelte';
  import LoadFailed from './components/LoadFailed.svelte';
  import Monitor from './pages/Monitor.svelte';
  import Dispatch from './pages/Dispatch.svelte';
  import Report24 from './pages/Report24.svelte';
  import AdminShell from './pages/admin/AdminShell.svelte';
  import LocationPanel from './pages/LocationPanel.svelte';
  import { live } from './lib/live.svelte';
  import { router } from './lib/router.svelte';
  import { applyTheme, effectiveTheme, saveTheme, storedTheme, validTheme, type Theme } from './lib/theme';

  const media = typeof matchMedia !== 'undefined' ? matchMedia('(prefers-color-scheme: dark)') : null;
  const queryTheme: Theme | null = router.route.query.tema === 'luminoasa' ? 'light' : router.route.query.tema === 'intunecata' ? 'dark' : null;
  // An explicit choice (URL or saved) wins; otherwise the theme follows the
  // system live, and the dispatch screen is dark.
  let chosen = $state<Theme | null>(queryTheme ?? validTheme(storedTheme()));
  let systemDark = $state(media?.matches ?? false);
  const theme = $derived(chosen ?? effectiveTheme(null, systemDark, router.route.name));

  $effect(() => applyTheme(theme));

  function setTheme(t: Theme) {
    chosen = t;
    saveTheme(t);
  }

  onMount(() => {
    live.start();
    const onScheme = (e: MediaQueryListEvent) => (systemDark = e.matches);
    media?.addEventListener('change', onScheme);
    return () => {
      live.stop();
      media?.removeEventListener('change', onScheme);
    };
  });

  const titles: Record<string, string> = {
    monitor: 'Monitorizare',
    location: 'Monitorizare',
    dispatch: 'Dispecerat',
    map: 'Hartă',
    stats: 'Statistici',
    report: 'Raport 24H',
    admin: 'Administrare',
    adminLocation: 'Administrare',
    adminPage: 'Administrare',
    notFound: 'Pagină inexistentă',
  };

  $effect(() => {
    document.title = `${titles[router.route.name]} · ${live.identity.name}`;
  });
</script>

{#if router.route.name !== 'dispatch'}<DemoBanner />{/if}
{#if router.route.name !== 'dispatch'}
  <Header
    name={live.identity.name}
    subtitle={live.identity.subtitle}
    logoVersion={live.identity.logo_version}
    {theme}
    onTheme={setTheme}
    minimal={router.route.name === 'admin' || router.route.name === 'adminLocation' || router.route.name === 'adminPage'}
  >
    {#snippet status()}<ConnectionBadge />{/snippet}
  </Header>
{/if}
<Announcer />
<ConfirmHost />
<main id="continut">
  {#if router.route.name === 'notFound'}
    <div class="page">
      <h1>Pagina nu există</h1>
      <p><a href="/">Înapoi la monitorizare</a></p>
    </div>
  {:else if router.route.name === 'monitor' || router.route.name === 'location'}
    <Monitor>
      {#snippet panel()}
        {#if router.route.name === 'location'}
          {#key router.route.params.id}
            <LocationPanel id={Number(router.route.params.id)} />
          {/key}
        {/if}
      {/snippet}
    </Monitor>
  {:else if router.route.name === 'map'}
    {#await import('./pages/MapPage.svelte') then m}<m.default />{:catch}<LoadFailed />{/await}
  {:else if router.route.name === 'stats'}
    {#await import('./pages/Stats.svelte') then m}<m.default />{:catch}<LoadFailed />{/await}
  {:else if router.route.name === 'report'}
    <Report24 />
  {:else if router.route.name === 'admin' || router.route.name === 'adminLocation' || router.route.name === 'adminPage'}
    <AdminShell />
  {:else if router.route.name === 'dispatch'}
    <Dispatch />
  {:else}
    <div class="page"><h1>{titles[router.route.name]}</h1></div>
  {/if}
</main>

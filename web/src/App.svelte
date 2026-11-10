<script lang="ts">
  import { onMount } from 'svelte';
  import Announcer from './components/Announcer.svelte';
  import ConnectionBadge from './components/ConnectionBadge.svelte';
  import Header from './components/Header.svelte';
  import Monitor from './pages/Monitor.svelte';
  import LocationPanel from './pages/LocationPanel.svelte';
  import { live } from './lib/live.svelte';
  import { router } from './lib/router.svelte';
  import { beeper, soundFor, soundPrefs } from './lib/sound.svelte';
  import { applyTheme, initialTheme, saveTheme, storedTheme, type Theme } from './lib/theme';

  const prefersDark = typeof matchMedia !== 'undefined' && matchMedia('(prefers-color-scheme: dark)').matches;
  const queryTheme = router.route.query.tema === 'luminoasa' ? 'light' : router.route.query.tema === 'intunecata' ? 'dark' : null;
  let theme = $state<Theme>(queryTheme ?? initialTheme(storedTheme(), prefersDark, router.route.name));

  $effect(() => applyTheme(theme));

  function setTheme(t: Theme) {
    theme = t;
    saveTheme(t);
  }

  const SOUND_ROUTES = ['monitor', 'location', 'dispatch', 'report'];

  onMount(() => {
    live.start();
    const off = live.onChange((prev, next) => {
      if (!SOUND_ROUTES.includes(router.route.name)) return;
      const kind = soundFor(prev?.state, next?.state, soundPrefs);
      if (kind) beeper.play(kind);
    });
    return () => {
      off();
      live.stop();
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

{#if router.route.name !== 'dispatch'}
  <Header name={live.identity.name} subtitle={live.identity.subtitle} {theme} onTheme={setTheme}>
    {#snippet status()}<ConnectionBadge />{/snippet}
  </Header>
{/if}
<Announcer />
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
  {:else}
    <div class="page"><h1>{titles[router.route.name]}</h1></div>
  {/if}
</main>

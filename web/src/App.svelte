<script lang="ts">
  import Header from './components/Header.svelte';
  import { router } from './lib/router.svelte';
  import { applyTheme, initialTheme, saveTheme, storedTheme, type Theme } from './lib/theme';

  const prefersDark = typeof matchMedia !== 'undefined' && matchMedia('(prefers-color-scheme: dark)').matches;
  let theme = $state<Theme>(initialTheme(storedTheme(), prefersDark, router.route.name));

  $effect(() => applyTheme(theme));

  function setTheme(t: Theme) {
    theme = t;
    saveTheme(t);
  }

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
    document.title = `${titles[router.route.name]} · NetWatch`;
  });
</script>

{#if router.route.name !== 'dispatch'}
  <Header name="NetWatch" subtitle="NETWORK MONITOR" {theme} onTheme={setTheme} />
{/if}
<main id="continut">
  {#if router.route.name === 'notFound'}
    <div class="page">
      <h1>Pagina nu există</h1>
      <p><a href="/">Înapoi la monitorizare</a></p>
    </div>
  {:else}
    <div class="page"><h1>{titles[router.route.name]}</h1></div>
  {/if}
</main>

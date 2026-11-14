<script>
  import { onMount } from 'svelte';

  const t = {
    bg: { title: 'Аварии във вашия район', pick: 'Изберете област', loading: 'Зареждане…', error: 'Грешка при зареждане', settlements: 'Населени места', providers: 'Доставчици', water: 'вода', power: 'ток' },
    en: { title: 'Outages in your area', pick: 'Choose a region', loading: 'Loading…', error: 'Could not load regions', settlements: 'Settlements', providers: 'Providers', water: 'water', power: 'power' }
  };
  let lang = $state('bg');
  let regions = $state(null);
  let failed = $state(false);
  let selected = $state('');
  const L = $derived(t[lang]);
  const region = $derived(regions?.find((r) => String(r.id) === selected));

  onMount(async () => {
    try {
      const res = await fetch('/api/regions');
      if (!res.ok) throw new Error(res.status);
      regions = await res.json();
    } catch {
      failed = true;
    }
  });
</script>

<main>
  <nav>
    <button class:on={lang === 'bg'} onclick={() => (lang = 'bg')}>BG</button>
    <button class:on={lang === 'en'} onclick={() => (lang = 'en')}>EN</button>
  </nav>
  <h1>{L.title}</h1>
  {#if failed}
    <p>{L.error}</p>
  {:else if !regions}
    <p>{L.loading}</p>
  {:else}
    <label>
      {L.pick}
      <select bind:value={selected}>
        <option value="" disabled>—</option>
        {#each regions as r}<option value={String(r.id)}>{r.name}</option>{/each}
      </select>
    </label>
    {#if region}
      <h2>{L.settlements}</h2>
      <ul>{#each region.settlements as s}<li>{s.name}</li>{/each}</ul>
      <h2>{L.providers}</h2>
      <ul>{#each region.providers as p}<li>{p.name} ({L[p.utility]})</li>{/each}</ul>
    {/if}
  {/if}
</main>

<style>
  :global(body) { margin: 0; font-family: system-ui, sans-serif; }
  main { max-width: 40rem; margin: 0 auto; padding: 1rem; }
  nav { text-align: right; }
  nav button.on { font-weight: bold; }
  select { display: block; width: 100%; margin-top: 0.5rem; padding: 0.6rem; font-size: 1rem; }
</style>

<script>
  import { onMount } from 'svelte';

  const t = {
    bg: { title: 'Аварии във вашия район', pick: 'Изберете област', loading: 'Зареждане…', error: 'Грешка при зареждане', settlements: 'Населени места', providers: 'Доставчици', water: 'вода', power: 'ток', cut: 'Прекъсване', disturbance: 'Възможни смущения', planned: 'планирано', emergency: 'аварийно', approx: '~', original: 'Оригинално съобщение', none: 'Няма планирани прекъсвания за следващите 7 дни.', whole: 'цялото населено място' },
    en: { title: 'Outages in your area', pick: 'Choose a region', loading: 'Loading…', error: 'Could not load regions', settlements: 'Settlements', providers: 'Providers', water: 'water', power: 'power', cut: 'Cut', disturbance: 'Possible disturbances', planned: 'planned', emergency: 'emergency', approx: '~', original: 'Original notice (Bulgarian)', none: 'No planned outages in the next 7 days.', whole: 'whole settlement' }
  };
  let lang = $state('bg');
  let regions = $state(null);
  let failed = $state(false);
  let selected = $state('');
  const L = $derived(t[lang]);
  const region = $derived(regions?.find((r) => String(r.id) === selected));
  let days = $state(null);

  $effect(() => {
    const id = selected;
    const l = lang;
    days = null;
    if (!id) return;
    fetch(`/api/regions/${id}/outages?lang=${l}`)
      .then((res) => res.json())
      .then((body) => (days = body.days))
      .catch(() => (failed = true));
  });

  const hm = (iso) => iso.slice(11, 16);
  const when = (o) => `${hm(o.start)}–${o.end_approximate ? L.approx : ''}${hm(o.end)}`;

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
      {#if days && days.length === 0}<p>{L.none}</p>{/if}
      {#each days ?? [] as day}
        <h2>{day.date}</h2>
        {#each day.settlements as s}
          <h3>{s.name}</h3>
          <ul class="outages">
            {#each s.outages as o}
              <li class={o.severity}>
                <strong>{when(o)}</strong> {L[o.utility]}, {L[o.severity]}, {L[o.kind]}
                <div>{o.places.length ? o.places.join('; ') : L.whole}</div>
                <details><summary>{L.original}</summary><pre lang="bg">{o.notice}</pre></details>
              </li>
            {/each}
          </ul>
        {/each}
      {/each}
    {/if}
  {/if}
</main>

<style>
  :global(body) { margin: 0; font-family: system-ui, sans-serif; }
  main { max-width: 40rem; margin: 0 auto; padding: 1rem; }
  nav { text-align: right; }
  nav button.on { font-weight: bold; }
  .outages { list-style: none; padding: 0; }
  .outages li { border-left: 4px solid #999; padding: 0.3rem 0.6rem; margin-bottom: 0.5rem; }
  .outages li.cut { border-color: #c00; }
  .outages li.disturbance { border-color: #e90; }
  pre { white-space: pre-wrap; font-family: inherit; }
  select { display: block; width: 100%; margin-top: 0.5rem; padding: 0.6rem; font-size: 1rem; }
</style>

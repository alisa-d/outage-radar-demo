<script>
  import { onMount } from 'svelte';
  const t = {
    bg: { title: 'Потвърждение на абонамента', consent: 'Съгласен съм с политиката за поверителност', go: 'Потвърди', ok: 'Абонаментът е потвърден.', bad: 'Връзката е невалидна или изтекла.' },
    en: { title: 'Confirm your subscription', consent: 'I agree to the privacy notice', go: 'Confirm', ok: 'Your subscription is confirmed.', bad: 'This link is invalid or expired.' }
  };
  let lang = $state('bg');
  let token = '';
  let consent = $state(false);
  let result = $state(null);
  const L = $derived(t[lang]);
  onMount(() => { token = new URLSearchParams(location.search).get('token') ?? ''; });
  async function confirm() {
    const res = await fetch('/api/verify', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ token, consent }) });
    result = res.ok ? 'ok' : 'bad';
  }
</script>

<main>
  <nav>
    <button onclick={() => (lang = 'bg')}>BG</button>
    <button onclick={() => (lang = 'en')}>EN</button>
  </nav>
  <h1>{L.title}</h1>
  {#if result}
    <p data-testid="result">{L[result]}</p>
  {:else}
    <label><input type="checkbox" bind:checked={consent} /> {L.consent}</label>
    <button disabled={!consent} onclick={confirm}>{L.go}</button>
  {/if}
</main>

<style>
  :global(body) { margin: 0; font-family: system-ui, sans-serif; }
  main { max-width: 40rem; margin: 0 auto; padding: 1rem; }
  nav { text-align: right; }
</style>

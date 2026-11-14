<script>
  import { onMount } from 'svelte';
  const t = {
    bg: { title: 'Отписване от известията', go: 'Отпиши ме', ok: 'Няма да получавате повече известия на този имейл.', bad: 'Връзката е невалидна.' },
    en: { title: 'Unsubscribe from alerts', go: 'Unsubscribe', ok: 'You will no longer get alerts on this email address.', bad: 'This link is invalid.' }
  };
  let lang = $state('bg');
  let token = '';
  let result = $state(null);
  const L = $derived(t[lang]);
  onMount(() => { token = new URLSearchParams(location.search).get('token') ?? ''; });
  async function unsubscribe() {
    const res = await fetch('/api/unsubscribe?token=' + encodeURIComponent(token), { method: 'POST' });
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
    <button onclick={unsubscribe}>{L.go}</button>
  {/if}
</main>

<style>
  :global(body) { margin: 0; font-family: system-ui, sans-serif; }
  main { max-width: 40rem; margin: 0 auto; padding: 1rem; }
  nav { text-align: right; }
</style>

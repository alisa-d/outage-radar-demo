<script>
  import { onMount } from 'svelte';
  import { siteKey, loadTurnstile } from '$lib/turnstile.js';
  const t = {
    bg: { title: 'Получавайте известия', email: 'Имейл', addr: 'Адрес', go: 'Абонирай ме', ok: 'Ако адресът е валиден, изпратихме имейл за потвърждение.', bad: 'Неуспешно абониране. Проверете имейла и опитайте пак.', noaddr: 'Първо проверете адрес на началната страница.', captcha: 'Потвърдете, че не сте робот.' },
    en: { title: 'Get alerts', email: 'Email', addr: 'Address', go: 'Subscribe', ok: 'If the address is valid, a verification email is on its way.', bad: 'Could not subscribe. Check the email and try again.', noaddr: 'Check an address on the home page first.', captcha: 'Please confirm you are not a robot.' }
  };
  let lang = $state('bg');
  const L = $derived(t[lang]);
  let email = $state('');
  let streetId = $state(0);
  let number = $state('');
  let token = $state('');
  let result = $state('');
  let widget = $state();

  onMount(() => {
    const q = new URLSearchParams(location.search);
    streetId = Number(q.get('street_id'));
    number = q.get('number') ?? '';
  });

  // the form (and its widget container) only exists once the address is known
  $effect(() => {
    if (!widget) return;
    loadTurnstile()
      .then((ts) => ts.render(widget, { sitekey: siteKey, callback: (tk) => (token = tk), 'expired-callback': () => (token = '') }))
      .catch(() => {});
  });

  async function submit(e) {
    e.preventDefault();
    if (!token) return (result = 'captcha');
    const res = await fetch('/api/subscriptions', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ turnstile_token: token, email, language: lang, addresses: [{ street_id: streetId, number }] })
    });
    result = res.ok ? 'ok' : 'bad';
    if (!res.ok) { window.turnstile?.reset(); token = ''; }
  }
</script>

<main>
  <nav>
    <button onclick={() => (lang = 'bg')}>BG</button>
    <button onclick={() => (lang = 'en')}>EN</button>
  </nav>
  <h1>{L.title}</h1>
  {#if !streetId || !number}
    <p>{L.noaddr}</p>
  {:else if result === 'ok'}
    <p data-testid="result">{L.ok}</p>
  {:else}
    <form onsubmit={submit}>
      <p>{L.addr}: <strong data-testid="addr">{number} (#{streetId})</strong></p>
      <label>{L.email} <input type="email" required bind:value={email} /></label>
      <div bind:this={widget}></div>
      <button type="submit">{L.go}</button>
      {#if result}<p data-testid="result">{L[result]}</p>{/if}
    </form>
  {/if}
</main>

<style>
  :global(body) { margin: 0; font-family: system-ui, sans-serif; }
  main { max-width: 40rem; margin: 0 auto; padding: 1rem; }
  nav { text-align: right; }
  input { display: block; width: 100%; margin: 0.4rem 0; padding: 0.6rem; font-size: 1rem; box-sizing: border-box; }
  button[type='submit'] { padding: 0.6rem 1rem; margin-top: 0.5rem; }
</style>

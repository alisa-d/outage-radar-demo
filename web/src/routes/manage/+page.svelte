<script>
  import { onMount } from 'svelte';
  import AddressPicker from '$lib/AddressPicker.svelte';
  const t = {
    bg: { title: 'Управление на абонамента', email: 'Имейл', send: 'Изпрати връзка', sent: 'Ако имейлът е абониран, връзката е на път.', bad: 'Връзката е невалидна или изтекла.', lang: 'Език', addrs: 'Адреси', channels: 'Контакти', add: 'Добави', remove: 'Премахване', exp: 'Изтегли данните ми', del: 'Изтрий всичко', sure: 'Да изтрием всичко завинаги?', gone: 'Данните ви са изтрити.', unver: '(непотвърден)', lastWarn: 'Това е последният контакт: всичко ще бъде изтрито. Продължаване?', added: 'Ако имейлът е нов, пратихме писмо за потвърждение.' },
    en: { title: 'Manage your subscription', email: 'Email', send: 'Send link', sent: 'If the email is subscribed, a link is on its way.', bad: 'This link is invalid or expired.', lang: 'Language', addrs: 'Addresses', channels: 'Contacts', add: 'Add', remove: 'Remove', exp: 'Download my data', del: 'Delete everything', sure: 'Delete everything permanently?', gone: 'Your data has been deleted.', unver: '(unverified)', lastWarn: 'This is your last contact: everything will be deleted. Continue?', added: 'If the email is new, we sent a confirmation message.' }
  };
  let lang = $state('bg');
  const L = $derived(t[lang]);
  let session = $state('');
  let v = $state(null);
  let msg = $state('');
  let email = $state('');
  let newEmail = $state('');
  let settlements = $state([]);

  const api = (method, path, body) =>
    fetch('/api/manage' + path, { method, headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + session }, body: body && JSON.stringify(body) });
  async function reload() {
    const res = await api('GET', '');
    if (res.ok) { v = await res.json(); lang = v.language; } else { v = null; msg = 'bad'; }
  }
  onMount(async () => {
    const token = new URLSearchParams(location.search).get('token');
    if (!token) return;
    history.replaceState(null, '', location.pathname); // single-use link: do not keep it in history
    const res = await fetch('/api/manage/session', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ token }) });
    if (!res.ok) { msg = 'bad'; return; }
    session = (await res.json()).session;
    await reload();
    const rs = await (await fetch('/api/regions')).json();
    settlements = rs.flatMap((r) => r.settlements);
  });
  async function request() {
    await fetch('/api/manage/request', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email }) });
    msg = 'sent';
  }
  async function setLang(l) { await api('PUT', '/language', { language: l }); await reload(); }
  async function removeAddress(id) { await api('DELETE', '/addresses/' + id); await reload(); }
  async function addAddress(a) {
    if (!a.streetId || !a.number) return;
    await api('POST', '/addresses', { street_id: a.streetId, number: a.number });
    await reload();
  }
  async function addChannel() { await api('POST', '/channels', { email: newEmail }); newEmail = ''; msg = 'added'; }
  async function removeChannel(id) {
    if (v.channels.filter((c) => c.verified).length <= 1 && !confirm(L.lastWarn)) return;
    await api('DELETE', '/channels/' + id);
    await reload();
    if (!v) msg = 'gone';
  }
  async function download() {
    const blob = await (await api('GET', '/export')).blob();
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = 'outage-radar-export.json';
    a.click();
  }
  async function eraseAll() {
    if (!confirm(L.sure)) return;
    await api('DELETE', '');
    v = null;
    msg = 'gone';
  }
</script>

<main>
  <nav>
    <button onclick={() => (lang = 'bg')}>BG</button>
    <button onclick={() => (lang = 'en')}>EN</button>
  </nav>
  <h1>{L.title}</h1>
  {#if msg}<p data-testid="msg">{L[msg]}</p>{/if}
  {#if v}
    <h2>{L.lang}</h2>
    <button disabled={v.language === 'bg'} onclick={() => setLang('bg')}>BG</button>
    <button disabled={v.language === 'en'} onclick={() => setLang('en')}>EN</button>
    <h2>{L.addrs}</h2>
    <ul>{#each v.addresses as a}<li>{a.street_id} {a.number} <button onclick={() => removeAddress(a.id)}>{L.remove}</button></li>{/each}</ul>
    <AddressPicker {settlements} {lang} onconfirm={addAddress} />
    <h2>{L.channels}</h2>
    <ul>{#each v.channels as c}<li>{c.email} {c.verified ? '' : L.unver} <button onclick={() => removeChannel(c.id)}>{L.remove}</button></li>{/each}</ul>
    <input type="email" bind:value={newEmail} placeholder={L.email} />
    <button onclick={addChannel}>{L.add}</button>
    <hr />
    <button onclick={download}>{L.exp}</button>
    <button onclick={eraseAll}>{L.del}</button>
  {:else if !session && msg !== 'gone'}
    <label>{L.email} <input type="email" bind:value={email} /></label>
    <button onclick={request}>{L.send}</button>
  {/if}
</main>

<style>
  :global(body) { margin: 0; font-family: system-ui, sans-serif; }
  main { max-width: 40rem; margin: 0 auto; padding: 1rem; }
  nav { text-align: right; }
</style>

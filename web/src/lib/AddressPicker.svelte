<script>
  import { onDestroy } from 'svelte';
  import L from 'leaflet';
  import 'leaflet/dist/leaflet.css';

  // settlements: [{name, ekatte, streets}], onconfirm: ({settlement, street?, streetId?, number?, lat, lon, approximate}) => void
  let { settlements, lang = 'bg', onconfirm } = $props();
  const t = {
    bg: { settlement: 'Населено място', street: 'Улица', number: 'Номер', find: 'Намери', confirm: 'Потвърди позицията', hint: 'Плъзнете маркера, ако позицията не е точна.', approx: 'Приблизителна позиция', none: 'Няма такъв номер' },
    en: { settlement: 'Settlement', street: 'Street', number: 'Number', find: 'Find', confirm: 'Confirm position', hint: 'Drag the pin if the position is off.', approx: 'Approximate position', none: 'No such number' }
  };
  const T = $derived(t[lang]);

  let ekatte = $state('');
  let query = $state('');
  let suggestions = $state([]);
  let street = $state(null);
  let num = $state('');
  let pin = $state(null); // {lat, lon, approximate}
  let missing = $state(false);
  let mapEl = $state();
  let map, marker;
  const settlement = $derived(settlements.find((s) => s.ekatte === ekatte));

  async function search() {
    street = null;
    if (!settlement?.streets || !query) return (suggestions = []);
    const res = await fetch(`/api/settlements/${ekatte}/streets?q=${encodeURIComponent(query)}`);
    suggestions = res.ok ? await res.json() : [];
  }

  function show(p) {
    pin = p;
    if (!map) {
      map = L.map(mapEl);
      L.tileLayer('https://tile.openstreetmap.org/{z}/{x}/{y}.png', { attribution: '© OpenStreetMap' }).addTo(map);
      marker = L.marker([p.lat, p.lon], { draggable: true }).addTo(map);
      marker.on('dragend', () => {
        const ll = marker.getLatLng();
        pin = { lat: ll.lat, lon: ll.lng, approximate: pin.approximate };
      });
    }
    marker.setLatLng([p.lat, p.lon]);
    map.setView([p.lat, p.lon], 18);
  }

  async function find() {
    missing = false;
    const res = await fetch(`/api/streets/${street.id}/numbers/${encodeURIComponent(num)}`);
    if (!res.ok) return (missing = true);
    show(await res.json());
  }

  function confirm() {
    onconfirm({ settlement: settlement.ekatte, street: street?.name, streetId: street?.id, number: num || undefined, lat: pin.lat, lon: pin.lon, approximate: pin.approximate });
  }

  onDestroy(() => map?.remove());
</script>

<label>
  {T.settlement}
  <select bind:value={ekatte} onchange={() => { query = ''; suggestions = []; street = null; }}>
    <option value="" disabled>—</option>
    {#each settlements as s}<option value={s.ekatte}>{s.name}</option>{/each}
  </select>
</label>

{#if settlement?.streets}
  <label>
    {T.street}
    <input bind:value={query} oninput={search} autocomplete="off" />
  </label>
  {#if !street}
    <ul>{#each suggestions as s}<li><button type="button" onclick={() => { street = s; query = s.name; suggestions = []; }}>{s.name}</button></li>{/each}</ul>
  {:else}
    <label>{T.number} <input bind:value={num} /></label>
    <button type="button" onclick={find} disabled={!num}>{T.find}</button>
    {#if missing}<p>{T.none}</p>{/if}
  {/if}
{:else if settlement}
  <!-- Settlement-level only: no street selection -->
  <button type="button" onclick={() => onconfirm({ settlement: settlement.ekatte })}>{T.confirm}</button>
{/if}

<div bind:this={mapEl} class="map" hidden={!pin}></div>
{#if pin}
  <p>{T.hint} {pin.approximate ? T.approx : ''}</p>
  <button type="button" onclick={confirm}>{T.confirm}</button>
{/if}

<style>
  .map { height: 20rem; margin-top: 1rem; }
  input, select { display: block; width: 100%; margin: 0.4rem 0; padding: 0.6rem; font-size: 1rem; box-sizing: border-box; }
</style>

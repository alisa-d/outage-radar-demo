<script>
  import { onDestroy } from 'svelte';
  import L from 'leaflet';
  import 'leaflet/dist/leaflet.css';

  // days: [{date, settlements: [{name, outages: [{id, area, unlocated, ...}]}]}] from the public Outages API
  let { days, lang = 'bg' } = $props();
  const t = {
    bg: { map: 'Карта на засегнатите райони', day: 'Ден', affected: 'Засегнат', possibly_affected: 'Възможно засегнат', unlocated: 'Без определено място', unlocatedHint: 'Не можахме да определим мястото на картата. Прочетете оригиналното съобщение.', original: 'Оригинално съобщение', details: 'Към аварията', water: 'вода', power: 'ток', cut: 'Прекъсване', disturbance: 'Възможни смущения' },
    en: { map: 'Map of affected areas', day: 'Day', affected: 'Affected', possibly_affected: 'Possibly affected', unlocated: 'Unlocated', unlocatedHint: 'We could not place these on the map. Please read the original notice.', original: 'Original notice (Bulgarian)', details: 'Go to outage', water: 'water', power: 'power', cut: 'Cut', disturbance: 'Possible disturbances' }
  };
  const T = $derived(t[lang]);
  const colour = { affected: '#c00', possibly_affected: '#e90' };

  let date = $state('');
  let el = $state();
  let map, layer;
  const day = $derived(days.find((d) => d.date === date) ?? days[0]);
  const outages = $derived((day?.settlements ?? []).flatMap((s) => s.outages));
  const unlocated = $derived(outages.filter((o) => o.unlocated));

  const esc = (s) => s.replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c]);
  const popup = (o) =>
    `<strong>${esc(T[o.utility])}, ${esc(T[o.severity])}</strong><br>${esc(o.start.slice(11, 16))}–${esc(o.end.slice(11, 16))}<br>` +
    `<a href="#outage-${o.id}">${esc(T.details)}</a><details><summary>${esc(T.original)}</summary><pre lang="bg" style="white-space:pre-wrap;max-width:16rem">${esc(o.notice)}</pre></details>`;

  $effect(() => {
    if (!el) return;
    if (!map) {
      map = L.map(el).setView([43.2141, 27.9147], 12);
      L.tileLayer('https://tile.openstreetmap.org/{z}/{x}/{y}.png', { attribution: '© OpenStreetMap' }).addTo(map);
      layer = L.featureGroup().addTo(map);
    }
    layer.clearLayers();
    for (const o of outages) {
      for (const f of o.area ?? []) {
        const c = f.properties.confidence;
        const style = { color: colour[c], weight: 6, opacity: 0.8, dashArray: c === 'possibly_affected' ? '4 8' : null };
        const g = L.geoJSON(f, { style, pointToLayer: (_, ll) => L.circleMarker(ll, { radius: 8, color: colour[c], fillColor: colour[c], fillOpacity: c === 'possibly_affected' ? 0.3 : 0.8 }) });
        g.bindPopup(popup(o)).addTo(layer);
      }
    }
    if (layer.getLayers().length) map.fitBounds(layer.getBounds(), { maxZoom: 16, padding: [20, 20] });
  });

  onDestroy(() => map?.remove());
</script>

<section data-testid="region-map">
  <h2>{T.map}</h2>
  {#if days.length}
    <label>{T.day}
      <select bind:value={date}>
        {#each days as d}<option value={d.date} selected={d.date === day.date}>{d.date}</option>{/each}
      </select>
    </label>
  {/if}
  <div class="legend">
    <span><i style="background:{colour.affected}"></i>{T.affected}</span>
    <span><i class="dash" style="border-color:{colour.possibly_affected}"></i>{T.possibly_affected}</span>
  </div>
  <div class="wrap">
    <div class="map" bind:this={el}></div>
    {#if unlocated.length}
      <aside data-testid="unlocated">
        <h3>{T.unlocated}</h3>
        <p>{T.unlocatedHint}</p>
        {#each unlocated as o}
          <div id={`unlocated-${o.id}`}>
            <a href={`#outage-${o.id}`}>{T[o.utility]}, {T[o.severity]}, {o.start.slice(11, 16)}–{o.end.slice(11, 16)}</a>
            <details><summary>{T.original}</summary><pre lang="bg">{o.notice}</pre></details>
          </div>
        {/each}
      </aside>
    {/if}
  </div>
</section>

<style>
  .wrap { display: flex; flex-wrap: wrap; gap: 1rem; }
  .map { flex: 2 1 20rem; height: 22rem; min-height: 16rem; }
  aside { flex: 1 1 14rem; min-width: 0; }
  .legend { display: flex; gap: 1rem; margin: 0.5rem 0; flex-wrap: wrap; }
  .legend i { display: inline-block; width: 1.5rem; height: 0.4rem; margin-right: 0.3rem; vertical-align: middle; }
  .legend i.dash { height: 0; border-top: 0.4rem dotted; }
  pre { white-space: pre-wrap; font-family: inherit; }
</style>

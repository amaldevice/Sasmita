<script lang="ts">
  // PROTOTYPE switcher bar. Not part of the design being evaluated.
  type Key = 'variant' | 'world' | 'theme' | 'lang';
  let { variant, world, theme, lang, set, replay }: { variant: string; world: string; theme: string; lang: string; set: (k: Key, v: string) => void; replay: () => void } = $props();

  const variants = [['A', 'Standar'], ['B', 'Workspace terbelah'], ['C', 'Halaman baca']];
  const worlds = [['papan', 'Papan Tulis Kelas'], ['koran', 'Infografis Koran'], ['canon', 'Standar kategori']];
  let idx = $derived(variants.findIndex(([k]) => k === variant));
  const cycle = (d: number) => set('variant', variants[(idx + d + variants.length) % variants.length][0]);

  function onkeydown(e: KeyboardEvent) {
    const el = e.target as HTMLElement;
    if (el.closest('input, textarea, [contenteditable]')) return;
    if (e.key === 'ArrowLeft') cycle(-1);
    if (e.key === 'ArrowRight') cycle(1);
  }
</script>


<div class="bar" role="toolbar" aria-label="Prototype switcher">
  <span class="label">PROTOTYPE</span>
  <select value={world} onchange={(e) => set('world', e.currentTarget.value)} aria-label="World">
    {#each worlds as [k, n] (k)}<option value={k}>{n}</option>{/each}
  </select>
  <button onclick={() => set('theme', theme === 'dark' ? 'light' : 'dark')}>{theme === 'dark' ? 'Gelap' : 'Terang'}</button>
  <button onclick={() => set('lang', lang === 'id' ? 'en' : 'id')}>{lang.toUpperCase()}</button>
  <button onclick={replay}>Putar ulang</button>
</div>

<style>
  .bar { position: fixed; left: 50%; top: 12px; translate: -50% 0; z-index: 99; display: flex; align-items: center; gap: 6px; padding: 6px 8px; border-radius: 999px; background: #0b0b0f; color: #fff; font: 600 13px system-ui, sans-serif; box-shadow: 0 8px 28px rgba(0, 0, 0, 0.45); border: 1px solid #ffffff22; }
  button, select { color: #fff; background: #ffffff14; border: 0; border-radius: 999px; padding: 5px 10px; font: inherit; cursor: pointer; }
  button:hover, select:hover { background: #ffffff26; }
  .label { padding: 0 6px; opacity: 0.6; }
  i { width: 1px; height: 18px; background: #ffffff33; margin: 0 4px; }
  select option { color: #000; }
</style>

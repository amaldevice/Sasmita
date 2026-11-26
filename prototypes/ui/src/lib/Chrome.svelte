<script lang="ts" module>
  // Small shared pieces: icons, sidebar, composer, visual skeleton.
  export const icons: Record<string, string> = {
    plus: 'M12 5v14M5 12h14',
    search: 'M11 5a6 6 0 1 0 0 12 6 6 0 0 0 0-12zM20 20l-4-4',
    folder: 'M3 7h7l2 2h9v10H3z',
    box: 'M4 4h16v16H4zM4 10h16',
    gear: 'M12 9a3 3 0 1 0 0 6 3 3 0 0 0 0-6zM12 2v3M12 19v3M2 12h3M19 12h3M5 5l2 2M17 17l2 2M5 19l2-2M17 7l2-2',
    attach: 'M21 11l-8.5 8.5a5 5 0 0 1-7-7L14 4a3.5 3.5 0 0 1 5 5l-8.5 8.5a2 2 0 0 1-3-3L15 7',
    up: 'M12 19V5M5 12l7-7 7 7',
    expand: 'M14 4h6v6M10 20H4v-6M20 4l-7 7M4 20l7-7',
  };
</script>

<script lang="ts">
  import type { Strings } from './i18n';
  let { part, s, compact = false }: { part: 'sidebar' | 'rail' | 'composer' | 'skeleton'; s: Strings; compact?: boolean } = $props();
  let focused = $state(false);
  let value = $state('');
</script>

{#snippet icon(name: string)}
  <svg class="ic" viewBox="0 0 24 24" aria-hidden="true"><path d={icons[name]} /></svg>
{/snippet}

{#if part === 'sidebar'}
  <aside class="side">
    <div class="brand">Sasmita</div>
    <nav>
      <button class="nav">{@render icon('plus')}{s.newChat}</button>
      <button class="nav">{@render icon('search')}{s.search}</button>
      <button class="nav">{@render icon('folder')}{s.projects}</button>
      <button class="nav">{@render icon('box')}{s.artifacts}</button>
    </nav>
    <div class="sec">{s.history}</div>
    {#each s.chats as chat, i (chat)}
      <button class={['chat', i === 0 && 'on']} title={i === 1 ? s.running : i === 2 ? s.waiting : undefined}>
        <span class="name">{chat}</span>
        {#if i === 1}<span class="dot run" aria-label={s.running}></span>{/if}
        {#if i === 2}<span class="dot wait" aria-label={s.waiting}></span>{/if}
      </button>
    {/each}
    <button class="nav foot">{@render icon('gear')}{s.settings}</button>
  </aside>
{:else if part === 'rail'}
  <nav class="rail">
    <div class="logo">S</div>
    {#each ['plus', 'search', 'folder', 'box'] as n (n)}<button class="rbtn" aria-label={n}>{@render icon(n)}</button>{/each}
    <button class="rbtn foot" aria-label={s.settings}>{@render icon('gear')}</button>
  </nav>
{:else if part === 'composer'}
  <div class={['composer', focused && 'focus', compact && 'compact']}>
    <textarea rows="1" placeholder={s.placeholder} bind:value onfocus={() => (focused = true)} onblur={() => (focused = false)}></textarea>
    <div class="chips">
      <button class="chip">{s.mode}<svg class="car" viewBox="0 0 24 24" aria-hidden="true"><path d="M7 10l5 5 5-5" /></svg></button>
      <button class="chip">{s.model}<svg class="car" viewBox="0 0 24 24" aria-hidden="true"><path d="M7 10l5 5 5-5" /></svg></button>
      <button class="chip icon" aria-label={s.attach}>{@render icon('attach')}</button>
      <button class={['send', value && 'ready']} aria-label="Send">{@render icon('up')}</button>
    </div>
  </div>
{:else}
  <div class="skel" aria-hidden="true">
    <div class="bar w30"></div>
    <div class="grid"><div class="box"></div><div class="box"></div></div>
    <div class="bar w60"></div>
  </div>
{/if}

<style>
  .ic { width: 18px; height: 18px; stroke: currentColor; fill: none; stroke-width: 1.8; stroke-linecap: round; stroke-linejoin: round; flex: none; }
  .side { background: var(--side); border-right: 1px solid var(--line); padding: 18px 12px; display: flex; flex-direction: column; gap: 2px; min-height: 0; overflow: auto; }
  .brand { font: 700 19px var(--display); letter-spacing: -0.01em; padding: 4px 10px 16px; }
  .nav, .chat { display: flex; gap: 12px; align-items: center; width: 100%; text-align: left; padding: 8px 10px; border-radius: var(--r-sm); font-weight: 500; transition: background 150ms var(--ease-out); }
  .nav:hover, .chat:hover { background: var(--raise); }
  .nav .ic { color: var(--mute); }
  .sec { color: var(--mute); font-size: 13px; font-weight: 600; padding: 18px 10px 6px; }
  .chat { color: var(--mute); font-size: 14.5px; padding: 7px 10px; }
  .chat.on { background: var(--raise); color: var(--text); }
  .name { flex: 1; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .dot { width: 8px; height: 8px; border-radius: 50%; flex: none; }
  .dot.run { background: var(--accent); animation: breathe 1.4s ease-in-out infinite; }
  .dot.wait { background: var(--attn); box-shadow: 0 0 0 0 var(--attn); animation: ping 1.8s var(--ease-out) infinite; }
  @keyframes breathe { 50% { opacity: 0.35; } }
  @keyframes ping { 70% { box-shadow: 0 0 0 6px transparent; } }
  .foot { margin-top: auto; }

  .rail { background: var(--side); border-right: 1px solid var(--line); display: flex; flex-direction: column; align-items: center; gap: 10px; padding: 16px 0; position: relative; }
  .logo { font: 700 22px var(--display); margin-bottom: 8px; }
  .rbtn { width: 40px; height: 40px; display: grid; place-items: center; border-radius: 10px; color: var(--mute); }
  .rbtn:hover { background: var(--raise); color: var(--text); }
  .railDot { position: absolute; top: 140px; right: 14px; }

  .composer { background: var(--surface); border: 1px solid var(--line); border-radius: var(--r-lg); padding: 12px 14px 10px 20px; box-shadow: 0 6px 20px -6px rgba(0, 0, 0, 0.45); transition: border-color 200ms var(--ease-out), box-shadow 200ms var(--ease-out); }
  .composer.focus { border-color: color-mix(in srgb, var(--accent) 60%, var(--line)); box-shadow: 0 6px 20px -6px rgba(0, 0, 0, 0.45), 0 0 0 3px color-mix(in srgb, var(--accent) 14%, transparent); }
  textarea { width: 100%; resize: none; background: none; border: 0; color: var(--text); font: inherit; outline: none; padding: 4px 0 8px; }
  textarea::placeholder { color: var(--mute); }
  .chips { display: flex; gap: 8px; align-items: center; }
  .chip { display: inline-flex; align-items: center; gap: 2px; border-radius: 6px; padding: 4px 6px 4px 8px; font-size: 14px; font-weight: 500; color: var(--mute); transition: background 150ms var(--ease-out), color 150ms var(--ease-out); }
  .chip:hover { color: var(--text); }
  .car { width: 16px; height: 16px; stroke: currentColor; fill: none; stroke-width: 2; }
  .chip:hover { background: var(--raise); }
  .chip.icon { padding: 4px 8px; display: grid; place-items: center; }
  .send { margin-left: auto; width: 32px; height: 32px; border-radius: 8px; display: grid; place-items: center; background: var(--raise); color: var(--mute); transition: background 200ms var(--ease-out), color 200ms var(--ease-out), transform 120ms var(--ease-out); }
  .send.ready { background: var(--text); color: var(--bg); }
  .send:active { transform: scale(0.92); }

  .skel { display: grid; gap: 14px; padding: 4px 0 12px; }
  .bar, .box { border-radius: 8px; background: linear-gradient(90deg, var(--raise) 0%, color-mix(in srgb, var(--raise) 50%, var(--text) 8%) 50%, var(--raise) 100%) 0 0 / 200% 100%; animation: shimmer 1.3s linear infinite; }
  .bar { height: 14px; } .w30 { width: 30%; } .w60 { width: 60%; }
  .grid { display: grid; grid-template-columns: 1.5fr 1fr; gap: 24px; }
  .box { height: 210px; }
  @keyframes shimmer { to { background-position: -200% 0; } }
</style>

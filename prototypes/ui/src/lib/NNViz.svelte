<script lang="ts">
  import { Tween } from 'svelte/motion';
  import { cubicOut } from 'svelte/easing';
  import { fade } from 'svelte/transition';
  import type { Strings } from './i18n';
  import { curve, lossAt, EPOCHS, reducedMotion } from './motion';

  type Phase = 'forward' | 'backward' | 'update';
  let { s, figures = false, big = false, autoplay = true }: { s: Strings; figures?: boolean; big?: boolean; autoplay?: boolean } = $props();

  let lr = $state(0.01);
  let epoch = $state(0);
  let playing = $state(false);
  let manualPhase = $state<Phase>('backward');
  const ys = new Tween(curve(0.01), { duration: 450, easing: cubicOut });

  const phases: Phase[] = ['forward', 'backward', 'update'];
  let phase = $derived<Phase>(playing ? phases[epoch % 3] : manualPhase);
  let loss = $derived(lossAt(epoch, lr));
  let drop = $derived(Math.round((1 - loss / lossAt(0, lr)) * 100));
  let hint = $derived(lr > 0.06 ? 'high' : lr < 0.004 ? 'low' : null);

  // Network geometry
  const cols = [[60, 50, 110, 170], [200, 40, 110, 180], [360, 80, 150]].map(([x, ...ysx]) => ysx.map((y) => ({ x, y })));
  const edges = cols.slice(0, -1).flatMap((c, i) => c.flatMap((a) => cols[i + 1].map((b) => ({ a, b, col: i }))));
  const hot = new Set(['60,110>200,110', '200,110>360,80', '200,180>360,150', '60,170>200,180']);
  const key = (e: (typeof edges)[number]) => `${e.a.x},${e.a.y}>${e.b.x},${e.b.y}`;

  // Loss chart geometry
  const W = 280, H = 190, X0 = 30, Y0 = 20;
  const px = (e: number) => X0 + (e / EPOCHS) * (W - X0 - 8);
  const py = (v: number) => Y0 + (1 - v) * (H - Y0 - 10);
  let path = $derived(ys.current.map((v, i) => `${i ? 'L' : 'M'}${px(i).toFixed(1)} ${py(v).toFixed(1)}`).join(''));

  function setLr(v: number) { lr = v; ys.target = curve(v); }
  function toggle() { if (epoch >= EPOCHS) epoch = 0; playing = !playing; }

  $effect(() => {
    if (!playing) return;
    const id = setInterval(() => { if (epoch >= EPOCHS) playing = false; else epoch += 1; }, 90);
    return () => clearInterval(id);
  });

  $effect(() => {
    if (!autoplay) return;
    if (reducedMotion()) { epoch = 12; return; }
    const id = setTimeout(() => (playing = true), 1400);
    return () => clearTimeout(id);
  });
</script>

<div class={['viz', big && 'big']}>
  <div class="ctrl">
    <div class="seg" role="tablist">
      {#each phases as p (p)}
        <button role="tab" aria-selected={phase === p} class={{ on: phase === p }} onclick={() => { playing = false; manualPhase = p; }}>{s.phases[p]}</button>
      {/each}
    </div>
    <label class="slider">
      <span>{s.lr}</span>
      <input type="range" min="0.001" max="0.1" step="0.001" value={lr} oninput={(e) => setLr(+e.currentTarget.value)} style:--pct={`${((lr - 0.001) / 0.099) * 100}%`} />
      <b class="num">{lr.toFixed(3).replace('.', s.dec)}</b>
    </label>
    <button class="play" onclick={toggle} aria-pressed={playing}>
      <svg viewBox="0 0 24 24" aria-hidden="true">{#if playing}<path d="M8 5v14M16 5v14" />{:else}<path d="M7 5l12 7-12 7z" />{/if}</svg>
      {playing ? s.pause : epoch >= EPOCHS ? s.reset : s.play}
    </button>
  </div>

  {#if hint}
    <p class="hint" transition:fade={{ duration: 180 }}>{hint === 'high' ? s.tooHigh : s.tooLow}</p>
  {/if}

  <div class="charts">
    <figure>
      {#if figures}<figcaption><b>{s.fig1}</b> {s.fig1t}</figcaption>{/if}
      <svg viewBox="0 0 440 220" class={['net', phase]}>
        {#each edges as e, i (key(e))}
          <line x1={e.a.x} y1={e.a.y} x2={e.b.x} y2={e.b.y} pathLength="1" class="edge draw" style:--d={`${120 + e.col * 260 + (i % 9) * 18}ms`} />
          {#if hot.has(key(e))}
            <line x1={e.a.x} y1={e.a.y} x2={e.b.x} y2={e.b.y} class="hot" />
          {/if}
        {/each}
        {#each cols as col, ci (ci)}
          {#each col as n (n.y)}
            <circle cx={n.x} cy={n.y} r={ci === 2 ? 15 : 13} class={['node', ci === 2 && n.y === 80 && 'out']} style:--d={`${ci * 260}ms`} />
          {/each}
        {/each}
        <text x="34" y="214">input</text><text x="176" y="214">hidden</text><text x="334" y="214">output</text>
        <text x="380" y="84" class="lossLabel num">{loss.toFixed(2).replace('.', s.dec)}</text>
      </svg>
    </figure>
    <figure>
      {#if figures}<figcaption><b>{s.fig2}</b> {s.fig2t}</figcaption>{/if}
      <svg viewBox="0 0 {W} 220" class="chart">
        <path d="M{X0} {Y0}V{H}H{W}" class="axis" />
        <path d={path} class="curve draw" pathLength="1" style:--d="700ms" />
        <circle cx={px(epoch)} cy={py(loss)} r="5" class="marker" />
        <text x={X0 + 4} y="14">loss</text><text x={W - 40} y="210">epoch</text>
      </svg>
    </figure>
  </div>

  <div class="legend"><span style:--c="var(--text)">{s.legend[0]}</span><span style:--c="var(--flow)">{s.legend[1]}</span><span style:--c="var(--accent)">{s.legend[2]}</span></div>

  <div class="metrics">
    <div><strong class="num">{loss.toFixed(2).replace('.', s.dec)}</strong>{s.lossNow}</div>
    <div><strong class="num">{epoch} / {EPOCHS}</strong>{s.epoch}</div>
    <div><strong class="num">{drop > 0 ? '−' : ''}{Math.abs(drop)}%</strong>{s.drop}</div>
  </div>
</div>

<style>
  .viz { --gap: 12px; }
  .ctrl { display: flex; gap: 14px; align-items: center; flex-wrap: wrap; margin-bottom: 8px; }
  .seg { display: flex; border: 1px solid var(--line); border-radius: 10px; overflow: hidden; }
  .seg button { padding: 7px 14px; font-weight: 600; font-size: 14px; color: var(--mute); transition: background 200ms var(--ease-out), color 200ms var(--ease-out); }
  .seg button.on { background: var(--raise); color: var(--accent); }
  .slider { flex: 1; min-width: 220px; display: flex; align-items: center; gap: 12px; font-size: 14px; color: var(--mute); }
  .slider b { color: var(--text); min-width: 3.2em; }
  input[type='range'] { flex: 1; appearance: none; height: 4px; border-radius: 2px; background: linear-gradient(var(--accent), var(--accent)) 0 / var(--pct) 100% no-repeat, var(--line); }
  input[type='range']::-webkit-slider-thumb { appearance: none; width: 18px; height: 18px; border-radius: 50%; background: var(--text); transition: transform 150ms var(--ease-out); }
  input[type='range']:active::-webkit-slider-thumb { transform: scale(1.25); }
  .play { display: inline-flex; gap: 6px; align-items: center; padding: 6px 14px 6px 10px; border-radius: 999px; background: var(--accent); color: var(--on-accent); font-weight: 700; font-size: 14px; transition: transform 120ms var(--ease-out); }
  .play:active { transform: scale(0.96); }
  .play svg { width: 16px; height: 16px; fill: currentColor; stroke: currentColor; stroke-width: 2.4; }
  .hint { font-size: 14px; color: var(--attn); margin-bottom: 4px; }

  .charts { display: grid; grid-template-columns: 1.5fr 1fr; gap: 24px; }
  .big .charts { grid-template-columns: 1.6fr 1fr; gap: 32px; }
  figure { min-width: 0; }
  figcaption { font-size: 14px; margin-bottom: 4px; }
  figcaption b { color: var(--attn); }
  svg { width: 100%; height: auto; overflow: visible; }
  text { fill: var(--mute); font: 12px var(--font); }

  .draw { stroke-dasharray: 1; stroke-dashoffset: 1; animation: draw 700ms var(--ease-out) var(--d, 0ms) forwards; }
  @keyframes draw { to { stroke-dashoffset: 0; } }
  .edge { stroke: var(--line); stroke-width: 1.3; }
  .hot { stroke: var(--flow); stroke-width: 2.6; stroke-dasharray: 6 8; opacity: 0; animation: appear 300ms 1100ms forwards, flow 900ms linear infinite; }
  .net.backward .hot { animation-direction: normal, reverse; }
  .net.update .hot { stroke: var(--accent); stroke-dasharray: none; animation: appear 300ms 1100ms forwards, pulse 700ms var(--ease-out) infinite alternate; }
  @keyframes flow { to { stroke-dashoffset: -28; } }
  @keyframes appear { to { opacity: 1; } }
  @keyframes pulse { from { opacity: 0.35; } to { opacity: 1; } }
  .node { fill: var(--bg); stroke: var(--text); stroke-width: 2; transform-box: fill-box; transform-origin: center; scale: 0; animation: pop 450ms var(--ease-out) var(--d) forwards; }
  .node.out { stroke: var(--accent); }
  .net.forward .node.out { animation: pop 450ms var(--ease-out) var(--d) forwards, glow 900ms ease-in-out 1.2s infinite alternate; }
  @keyframes pop { to { scale: 1; } }
  @keyframes glow { to { stroke-width: 4; } }
  .lossLabel { fill: var(--accent); font-weight: 700; }
  .axis { stroke: var(--line); fill: none; }
  .curve { stroke: var(--accent); stroke-width: 2.6; fill: none; stroke-linejoin: round; }
  .marker { fill: var(--accent); transition: cx 90ms linear, cy 90ms linear; filter: drop-shadow(0 0 6px color-mix(in srgb, var(--accent) 60%, transparent)); }

  .legend { display: flex; gap: 18px; flex-wrap: wrap; font-size: 13.5px; color: var(--mute); margin: 6px 0 14px; }
  .legend span::before { content: ''; display: inline-block; width: 10px; height: 10px; border-radius: 2px; margin-right: 6px; vertical-align: -1px; background: var(--c); }
  .metrics { display: flex; gap: 40px; flex-wrap: wrap; }
  .metrics div { font-size: 13.5px; color: var(--mute); }
  .metrics strong { display: block; font: 700 34px/1.1 var(--display); color: var(--text); letter-spacing: -0.02em; }
</style>

<script lang="ts">
  import { Tween } from 'svelte/motion';
  import { cubicOut } from 'svelte/easing';
  import { fade } from 'svelte/transition';
  import type { Strings } from './i18n';
  import { curve, lossAt, EPOCHS, reducedMotion } from './motion';

  let { s, stack = false, big = false, autoplay = true }: { s: Strings; stack?: boolean; big?: boolean; autoplay?: boolean } = $props();

  let lr = $state(0.01);
  let epoch = $state(0);
  let playing = $state(false);
  let picked = $state(2); // which step of the cycle is shown when paused
  const ys = new Tween(curve(0.01), { duration: 450, easing: cubicOut });

  // The four steps of one training iteration; order carries meaning, so they are numbered.
  let step = $derived(playing ? epoch % 4 : picked);
  let loss = $derived(lossAt(epoch, lr));
  let first = $derived(lossAt(0, lr));
  let hint = $derived(lr > 0.06 ? 'high' : lr < 0.004 ? 'low' : null);
  const f = (n: number) => n.toFixed(2).replace('.', s.dec);

  const cols = [[60, 50, 110, 170], [200, 40, 110, 180], [360, 80, 150]].map(([x, ...yy]) => yy.map((y) => ({ x, y })));
  const edges = cols.slice(0, -1).flatMap((c, i) => c.flatMap((a) => cols[i + 1].map((b) => ({ a, b, col: i }))));
  const hot = new Set(['60,110>200,110', '200,110>360,80', '200,180>360,150', '60,170>200,180']);
  const key = (e: (typeof edges)[number]) => `${e.a.x},${e.a.y}>${e.b.x},${e.b.y}`;

  const W = 300, H = 200, X0 = 8, Y0 = 16;
  const px = (e: number) => X0 + (e / EPOCHS) * (W - X0 - 8);
  const py = (v: number) => Y0 + (1 - v) * (H - Y0 - 24);
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

<div class={['viz', big && 'big', stack && 'stack']}>
  <ol class="cycle">
    {#each s.cycle as label, i (label)}
      <li><button class={{ on: step === i }} onclick={() => { playing = false; picked = i; }}><span class="n num">{i + 1}</span>{label}</button></li>
    {/each}
  </ol>

  <div class="charts">
    <svg viewBox="0 0 490 220" class={['net', `s${step}`]} role="img" aria-label={s.fig1t}>
      {#each edges as e, i (key(e))}
        <line x1={e.a.x} y1={e.a.y} x2={e.b.x} y2={e.b.y} pathLength="1" class="edge draw" style:--d={`${120 + e.col * 260 + (i % 9) * 18}ms`} />
        {#if hot.has(key(e))}<line x1={e.a.x} y1={e.a.y} x2={e.b.x} y2={e.b.y} class="hot" />{/if}
      {/each}
      {#each cols as col, ci (ci)}
        {#each col as n (n.y)}
          <circle cx={n.x} cy={n.y} r={ci === 2 ? 15 : 13} class={['node', ci === 2 && n.y === 80 && 'out']} style:--d={`${ci * 260}ms`} />
        {/each}
      {/each}
      <text x="60" y="212" text-anchor="middle">input</text><text x="200" y="212" text-anchor="middle">hidden</text><text x="360" y="212" text-anchor="middle">output</text>
      <text x="382" y="78" class="tag">{s.noteWrong}</text>
      <text x="382" y="94" class="val num">loss {f(loss)}</text>
      {#if step === 2}<text x="250" y="128" class="flowTag" in:fade={{ duration: 150 }}>← {s.noteBack}</text>{/if}
    </svg>

    <svg viewBox="0 0 {W} {H}" class="chart" role="img" aria-label={s.fig2t}>
      {#each [0.25, 0.5, 0.75] as g (g)}<line x1={X0} x2={W - 8} y1={py(g)} y2={py(g)} class="grid" />{/each}
      <path d={path} class="curve draw" pathLength="1" style:--d="700ms" />
      <line x1={px(epoch)} x2={px(epoch)} y1={Y0} y2={H - 24} class="now" />
      <circle cx={px(epoch)} cy={py(loss)} r="5" class="marker" />
      <text x={X0} y="10">loss</text>
      <text x={W - 8} y={H - 6} text-anchor="end">epoch</text>
      <text x={X0} y={H - 6} class="num">0</text>
    </svg>
  </div>

  <p class="summary num">
    {s.summary(String(epoch), f(first), f(loss), loss <= first ? s.down : s.up)}
    {#if hint}<span class="hint" transition:fade={{ duration: 180 }}>{hint === 'high' ? s.tooHigh : s.tooLow}</span>{/if}
  </p>

  <div class="ctrl">
    <label class="slider">
      <span>{s.lr}</span>
      <input type="range" min="0.001" max="0.1" step="0.001" value={lr} oninput={(e) => setLr(+e.currentTarget.value)} style:--pct={`${((lr - 0.001) / 0.099) * 100}%`} />
      <output class="num">{lr.toFixed(3).replace('.', s.dec)}</output>
    </label>
    <button class="play" onclick={toggle} aria-pressed={playing}>
      <svg viewBox="0 0 24 24" aria-hidden="true">{#if playing}<path d="M8 5v14M16 5v14" />{:else}<path d="M7 5l12 7-12 7z" />{/if}</svg>
      {playing ? s.pause : epoch >= EPOCHS ? s.again : s.trainN}
    </button>
  </div>
</div>

<style>
  .cycle { list-style: none; padding: 0; display: grid; grid-template-columns: repeat(4, 1fr); border-top: 1px solid var(--line); margin-bottom: 14px; }
  .cycle button { width: 100%; text-align: left; display: flex; gap: 8px; align-items: baseline; padding: 10px 0 4px; font-size: 14.5px; font-weight: 500; color: var(--mute); position: relative; transition: color 200ms var(--ease-out); }
  .cycle button::before { content: ''; position: absolute; left: 0; right: 12px; top: -1px; height: 2px; background: var(--accent); scale: 0 1; transform-origin: left; transition: scale 260ms var(--ease-out); }
  .cycle button.on { color: var(--text); }
  .cycle button.on::before { scale: 1 1; }
  .n { font-size: 12.5px; color: var(--mute); }
  .on .n { color: var(--accent); }

  .charts { display: grid; grid-template-columns: 1.5fr 1fr; gap: 28px; align-items: end; }
  .stack .charts { grid-template-columns: 1fr; gap: 12px; }
  .stack .chart { height: 180px; }
  .big .charts { gap: 40px; }
  svg { width: 100%; height: auto; overflow: visible; display: block; }
  text { fill: var(--mute); font: 12.5px var(--font); }
  .tag { fill: var(--text); font-weight: 500; }
  .val { fill: var(--accent); font-weight: 700; }
  .flowTag { fill: var(--flow); font-weight: 500; }

  .draw { stroke-dasharray: 1; stroke-dashoffset: 1; animation: draw 700ms var(--ease-out) var(--d, 0ms) forwards; }
  @keyframes draw { to { stroke-dashoffset: 0; } }
  .edge { stroke: var(--line); stroke-width: 1.3; }
  .hot { stroke: var(--flow); stroke-width: 2.6; stroke-dasharray: 6 8; opacity: 0; transition: opacity 250ms var(--ease-out); animation: flow 900ms linear infinite; }
  .net.s0 .hot { opacity: 1; animation-direction: reverse; }
  .net.s2 .hot { opacity: 1; }
  .net.s3 .hot { opacity: 1; stroke: var(--accent); stroke-dasharray: none; animation: pulse 600ms var(--ease-out) infinite alternate; }
  @keyframes flow { to { stroke-dashoffset: 28; } }
  @keyframes pulse { from { opacity: 0.3; } }
  .node { fill: var(--bg); stroke: var(--text); stroke-width: 2; transform-box: fill-box; transform-origin: center; scale: 0; animation: pop 450ms var(--ease-out) var(--d) forwards; }
  .node.out { stroke: var(--accent); }
  .net.s1 .node.out { stroke-width: 4; transition: stroke-width 200ms; }
  @keyframes pop { to { scale: 1; } }
  .grid { stroke: var(--line); stroke-dasharray: 2 4; }
  .curve { stroke: var(--accent); stroke-width: 2.6; fill: none; stroke-linejoin: round; }
  .now { stroke: color-mix(in srgb, var(--accent) 35%, transparent); }
  .marker { fill: var(--accent); }

  .summary { margin: 14px 0 10px; max-width: 64ch; }
  .hint { display: block; color: var(--attn); font-size: 14.5px; }
  .ctrl { display: flex; gap: 20px; align-items: center; flex-wrap: wrap; }
  .slider { flex: 1; min-width: 240px; display: flex; align-items: center; gap: 12px; font-size: 14.5px; color: var(--mute); }
  output { color: var(--text); font-weight: 700; min-width: 3.4em; }
  input[type='range'] { flex: 1; appearance: none; height: 3px; background: linear-gradient(var(--accent), var(--accent)) 0 / var(--pct) 100% no-repeat, var(--line); }
  input[type='range']::-webkit-slider-thumb { appearance: none; width: 16px; height: 16px; border-radius: 50%; background: var(--text); box-shadow: 0 1px 3px rgba(0, 0, 0, 0.4); transition: scale 150ms var(--ease-out); }
  input[type='range']:active::-webkit-slider-thumb { scale: 1.25; }
  .play { display: inline-flex; gap: 8px; align-items: center; padding: 7px 14px 7px 10px; border-radius: 6px; border: 1px solid var(--accent); color: var(--accent); font-weight: 700; font-size: 14.5px; transition: background 150ms var(--ease-out), scale 120ms var(--ease-out); }
  .play:hover { background: color-mix(in srgb, var(--accent) 12%, transparent); }
  .play:active { scale: 0.97; }
  .play svg { width: 14px; height: 14px; fill: currentColor; stroke: currentColor; stroke-width: 2.4; }
</style>

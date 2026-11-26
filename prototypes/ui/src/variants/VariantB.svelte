<script lang="ts">
  // B: split workspace. Narrow chat on the left, a persistent artifact panel on the right.
  import { fly, fade } from 'svelte/transition';
  import Chrome from '../lib/Chrome.svelte';
  import NNViz from '../lib/NNViz.svelte';
  import Activity from '../lib/Activity.svelte';
  import StreamText from '../lib/StreamText.svelte';
  import Approval from '../lib/Approval.svelte';
  import type { Strings } from '../lib/i18n';
  import type { AnswerRun, Token } from '../lib/run.svelte';

  let { s, run, tokens, world }: { s: Strings; run: AnswerRun; tokens: Token[]; world: string } = $props();
  let tab = $state<'preview' | 'code' | 'versions'>('preview');
</script>

<div class="app">
  <Chrome part="rail" {s} />
  <section class="chat">
    <header><b>{s.title}</b></header>
    <div class="scroll">
      <div class="user" in:fly={{ y: 10, duration: 300 }}>{s.ask}</div>
      <Activity {s} {run} />
      {#if run.stage >= 3}
        <button class="ref" in:fly={{ x: -10, duration: 360 }}>
          <span class="pulse" aria-hidden="true"></span>
          <span><b>{s.heading}</b><small>{s.live} · v1</small></span>
        </button>
      {/if}
      {#if run.stage >= 4}<StreamText {tokens} shown={run.words} {world} />{/if}
      <Approval {s} {run} />
    </div>
    <div class="dock"><Chrome part="composer" {s} compact /></div>
  </section>
  <section class="panel">
    <header>
      <div class="tabs" role="tablist">
        {#each ['preview', 'code', 'versions'] as const as k (k)}
          <button role="tab" aria-selected={tab === k} class={{ on: tab === k }} onclick={() => (tab = k)}>{k === 'preview' ? 'Preview' : k === 'code' ? 'Kode' : 'Versi'}</button>
        {/each}
        <span class="ind" style:--i={['preview', 'code', 'versions'].indexOf(tab)}></span>
      </div>
      <span class="meta">{s.mode} · {s.model}</span>
    </header>
    <div class="stage">
      {#if run.stage < 3}
        <div class="empty" out:fade={{ duration: 150 }}>
          {#if run.stage === 2}<Chrome part="skeleton" {s} />{:else}<p>{s.steps[0]}…</p>{/if}
        </div>
      {:else if tab === 'preview'}
        <div class="art" in:fly={{ y: 14, duration: 500 }}>
          <h2>{s.heading}</h2>
          <NNViz {s} big />
        </div>
      {:else if tab === 'code'}
        <pre in:fade>{`<script>\n  let lr = 0.01;\n  const loss = (e) => 0.06 + 0.94 * Math.exp(-e * lr * 6);\n</script>\n\n<input type="range" bind:value={lr} />\n<LossChart {loss} />`}</pre>
      {:else}
        <ol class="vers" in:fade><li class="on">v1 · {s.steps[1]}</li></ol>
      {/if}
    </div>
  </section>
</div>

<style>
  .app { display: grid; grid-template-columns: 64px minmax(360px, 440px) 1fr; height: 100vh; }
  header { height: 56px; flex: none; display: flex; align-items: center; justify-content: space-between; gap: 10px; padding: 0 22px; border-bottom: 1px solid var(--line); }
  .chat { display: flex; flex-direction: column; min-height: 0; border-right: 1px solid var(--line); position: relative; }
  .scroll { flex: 1; overflow: auto; padding: 22px 22px 170px; font-size: 15.5px; }
  .user { margin-left: auto; width: max-content; max-width: 88%; background: var(--user); padding: 10px 16px; border-radius: 18px 18px 4px 18px; }
  .ref { display: flex; gap: 12px; align-items: center; width: 100%; text-align: left; padding: 12px 14px; margin: 4px 0 14px; border: 1px solid var(--line); border-radius: var(--r-md); background: var(--surface); }
  .ref b { display: block; font-size: 14.5px; line-height: 1.35; }
  .ref small { color: var(--mute); }
  .pulse { width: 10px; height: 10px; border-radius: 50%; background: var(--ok); flex: none; animation: breathe 1.6s ease-in-out infinite; }
  @keyframes breathe { 50% { opacity: 0.3; } }
  .dock { position: absolute; left: 16px; right: 16px; bottom: 16px; }
  .panel { display: flex; flex-direction: column; min-width: 0; background: color-mix(in srgb, var(--side) 60%, var(--bg)); }
  .tabs { position: relative; display: flex; gap: 4px; }
  .tabs button { width: 84px; padding: 6px 0; font-weight: 600; font-size: 14px; color: var(--mute); z-index: 1; transition: color 200ms var(--ease-out); }
  .tabs button.on { color: var(--text); }
  .ind { position: absolute; inset: 0 auto 0 0; width: 84px; border-radius: 8px; background: var(--raise); translate: calc(var(--i) * 88px) 0; transition: translate 300ms var(--ease-out); }
  .meta { color: var(--mute); font-size: 13.5px; }
  .stage { flex: 1; overflow: auto; padding: 32px 44px; }
  .art h2 { font: 700 32px/1.2 var(--display); letter-spacing: -0.02em; margin-bottom: 18px; max-width: 24ch; text-wrap: balance; }
  .empty { color: var(--mute); }
  pre { font: 14px/1.6 ui-monospace, Menlo, monospace; background: var(--surface); border: 1px solid var(--line); border-radius: var(--r-md); padding: 18px; }
  .vers { list-style: none; padding: 0; }
  .vers li { padding: 10px 14px; border-radius: 8px; background: var(--raise); }
</style>

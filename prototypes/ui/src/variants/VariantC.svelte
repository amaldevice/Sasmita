<script lang="ts">
  // C: reading page. Icon rail, wide document answer with numbered figures, floating composer.
  import { fly, fade } from 'svelte/transition';
  import Chrome from '../lib/Chrome.svelte';
  import NNViz from '../lib/NNViz.svelte';
  import Activity from '../lib/Activity.svelte';
  import StreamText from '../lib/StreamText.svelte';
  import Approval from '../lib/Approval.svelte';
  import type { Strings } from '../lib/i18n';
  import type { AnswerRun, Token } from '../lib/run.svelte';

  let { s, run, tokens, world }: { s: Strings; run: AnswerRun; tokens: Token[]; world: string } = $props();
  let scrolled = $state(false);
</script>

<div class="app">
  <Chrome part="rail" {s} />
  <main onscroll={(e) => (scrolled = e.currentTarget.scrollTop > 120)}>
    <header class={{ solid: scrolled }}>
      <b>{s.title}</b>
      {#if scrolled}<span in:fade={{ duration: 150 }}>· {s.heading}</span>{/if}
    </header>
    <article>
      <p class="q" in:fade={{ duration: 300 }}><span>{s.ask}</span></p>
      <Activity {s} {run} />
      {#if run.stage >= 3}
        <h1 in:fly={{ y: 14, duration: 600 }}>{s.heading}</h1>
      {/if}
      <div class="fig">
        {#if run.stage === 2}<div out:fade={{ duration: 120 }}><Chrome part="skeleton" {s} /></div>{/if}
        {#if run.stage >= 3}<div class="rule" in:fade={{ duration: 200 }}><NNViz {s} figures /></div>{/if}
      </div>
      {#if run.stage >= 3}<p class="src" in:fade={{ delay: 900 }}>{s.source}</p>{/if}
      {#if run.stage >= 4}<StreamText {tokens} shown={run.words} {world} columns />{/if}
      <div class="follow"><Approval {s} {run} /></div>
    </article>
    <div class="dock"><Chrome part="composer" {s} /></div>
  </main>
</div>

<style>
  .app { display: grid; grid-template-columns: 64px 1fr; height: 100vh; }
  main { overflow: auto; position: relative; }
  header { position: sticky; top: 0; z-index: 2; height: 56px; display: flex; align-items: center; gap: 8px; padding: 0 40px; font-size: 14px; color: var(--mute); transition: background 250ms var(--ease-out), border-color 250ms var(--ease-out); border-bottom: 1px solid transparent; }
  header b { color: var(--text); }
  header.solid { background: color-mix(in srgb, var(--bg) 88%, transparent); backdrop-filter: blur(10px); border-color: var(--line); }
  article { max-width: 1040px; margin: 0 auto; padding: 24px 40px 200px; }
  .q { color: var(--mute); font-size: 15px; }
  .q span { color: var(--text); font-weight: 600; }
  h1 { font: 700 clamp(36px, 4vw, 52px)/1.08 var(--display); letter-spacing: -0.02em; max-width: 20ch; margin: 4px 0 22px; text-wrap: balance; }
  .rule { border-top: 2px solid var(--text); padding-top: 14px; animation: grow 700ms var(--ease-out); transform-origin: left; }
  @keyframes grow { from { clip-path: inset(0 100% 0 0); } to { clip-path: inset(0 0 0 0); } }
  .src { font-size: 12.5px; color: var(--mute); padding: 10px 0 22px; border-bottom: 1px solid var(--line); margin-bottom: 22px; }
  .follow { max-width: 680px; margin-left: auto; }
  .dock { position: sticky; bottom: 20px; max-width: 820px; margin: -160px auto 0; padding: 0 24px; }
</style>

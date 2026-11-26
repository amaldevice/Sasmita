<script lang="ts">
  // A: category standard layout. Sidebar, centre column, visual inline without a frame.
  import { fly, fade } from 'svelte/transition';
  import Chrome from '../lib/Chrome.svelte';
  import NNViz from '../lib/NNViz.svelte';
  import Activity from '../lib/Activity.svelte';
  import StreamText from '../lib/StreamText.svelte';
  import Approval from '../lib/Approval.svelte';
  import type { Strings } from '../lib/i18n';
  import type { AnswerRun, Token } from '../lib/run.svelte';

  let { s, run, tokens, world }: { s: Strings; run: AnswerRun; tokens: Token[]; world: string } = $props();
</script>

<div class="app">
  <Chrome part="sidebar" {s} />
  <main>
    <header><b>{s.title}</b><span>{s.mode}</span></header>
    <div class="scroll">
      <div class="col">
        <div class="user" in:fly={{ y: 10, duration: 300 }}>{s.ask}</div>
        <Activity {s} {run} />
        {#if run.stage === 2}
          <div out:fade={{ duration: 120 }}><Chrome part="skeleton" {s} /></div>
        {/if}
        {#if run.stage >= 3}
          <h2 in:fly={{ y: 8, duration: 420 }}>{s.heading}</h2>
          <div in:fade={{ duration: 200 }}><NNViz {s} /></div>
        {/if}
        {#if run.stage >= 4}<StreamText {tokens} shown={run.words} {world} />{/if}
        {#if run.stage >= 5}<p class="usage" in:fade>{s.usage}</p>{/if}
        <Approval {s} {run} />
      </div>
    </div>
    <div class="dock"><Chrome part="composer" {s} /></div>
  </main>
</div>

<style>
  .app { display: grid; grid-template-columns: 272px 1fr; height: 100vh; }
  main { display: flex; flex-direction: column; min-height: 0; position: relative; }
  header { height: 56px; flex: none; display: flex; align-items: center; gap: 10px; padding: 0 28px; border-bottom: 1px solid var(--line); }
  header span { color: var(--mute); font-size: 14px; }
  .scroll { flex: 1; overflow: auto; padding: 26px 24px 170px; }
  .col { max-width: 760px; margin: 0 auto; }
  .user { margin-left: auto; width: max-content; max-width: 70%; background: var(--user); padding: 12px 18px; border-radius: 18px 18px 4px 18px; }
  h2 { font: 700 30px/1.2 var(--display); letter-spacing: -0.02em; margin-bottom: 14px; text-wrap: balance; }
  .usage { font-size: 13px; color: var(--mute); }
  .dock { position: absolute; left: 0; right: 0; bottom: 20px; padding: 0 24px; }
  .dock > :global(*) { max-width: 800px; margin: 0 auto; }
</style>

<script lang="ts">
  import { slide } from 'svelte/transition';
  import type { Strings } from './i18n';
  import type { AnswerRun } from './run.svelte';

  let { s, run }: { s: Strings; run: AnswerRun } = $props();
  let open = $state(false);
  let done = $derived(run.stage >= 5);
  let current = $derived(run.stage >= 4 ? 2 : run.stage >= 2 ? 1 : 0);
</script>

{#if run.stage >= 1}
  <div class="act">
    <button class="line" onclick={() => (open = !open)} aria-expanded={open}>
      {#if done}
        <svg class="check" viewBox="0 0 24 24" aria-hidden="true"><path d="M5 12.5l4.5 4.5L19 7.5" /></svg>
        <span>{s.worked(run.seconds)}</span>
      {:else}
        <span class="spin" aria-hidden="true"></span>
        {#key current}<span class="label">{s.steps[current]}</span>{/key}
      {/if}
      <svg class={['chev', open && 'open']} viewBox="0 0 24 24" aria-hidden="true"><path d="M9 6l6 6-6 6" /></svg>
    </button>
    {#if open}
      <ol transition:slide={{ duration: 220 }}>
        {#each s.steps as step, i (step)}
          <li class={{ done: done || i < current, now: !done && i === current }}>{step}{#if s.tools[i]}<code>{s.tools[i]}</code>{/if}</li>
        {/each}
      </ol>
    {/if}
  </div>
{/if}

<style>
  .act { margin: 22px 0 12px; color: var(--mute); font-size: 14.5px; }
  .line { display: inline-flex; gap: 8px; align-items: center; color: var(--mute); padding: 2px 0; }
  .line:hover { color: var(--text); }
  .label { animation: in 260ms var(--ease-out); }
  @keyframes in { from { opacity: 0; transform: translateY(4px); } }
  .spin { width: 14px; height: 14px; border-radius: 50%; border: 2px solid var(--line); border-top-color: var(--accent); animation: spin 800ms linear infinite; }
  @keyframes spin { to { transform: rotate(360deg); } }
  .check { width: 16px; height: 16px; stroke: var(--ok); fill: none; stroke-width: 2.4; stroke-dasharray: 24; stroke-dashoffset: 24; animation: tick 400ms var(--ease-out) forwards; }
  @keyframes tick { to { stroke-dashoffset: 0; } }
  .chev { width: 14px; height: 14px; stroke: currentColor; fill: none; stroke-width: 2; transition: rotate 200ms var(--ease-out); }
  .chev.open { rotate: 90deg; }
  ol { list-style: none; padding: 8px 0 0 4px; display: grid; gap: 6px; border-left: 1px solid var(--line); margin: 6px 0 0 6px; }
  li { padding-left: 14px; position: relative; }
  li::before { content: ''; position: absolute; left: -5px; top: 9px; width: 9px; height: 9px; border-radius: 50%; border: 2px solid var(--line); background: var(--bg); }
  li.done::before { background: var(--ok); border-color: var(--ok); }
  li.now { color: var(--text); }
  code { font: 12px var(--mono); margin-left: 8px; padding: 1px 6px; border-radius: 4px; background: var(--raise); color: var(--mute); }
  li.now::before { border-color: var(--accent); border-style: dashed; animation: spin 1.6s linear infinite; }
</style>

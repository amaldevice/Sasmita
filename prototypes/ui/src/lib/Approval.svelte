<script lang="ts">
  import { fly, fade } from 'svelte/transition';
  import type { Strings } from './i18n';
  import type { AnswerRun } from './run.svelte';

  let { s, run }: { s: Strings; run: AnswerRun } = $props();
</script>

{#if run.stage >= 6}
  <div class="user" in:fly={{ y: 12, duration: 320 }}>{s.ask2}</div>
  {#if run.stage === 6}
    <div class="card" in:fly={{ y: 16, duration: 400, delay: 500 }} out:fade={{ duration: 150 }} role="group" aria-label={s.approvalT}>
      <div class="head">
        <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 3l8 4v5c0 4.5-3.4 8.3-8 9-4.6-.7-8-4.5-8-9V7z" /></svg>
        <b>{s.approvalT}</b>
      </div>
      <p><code>{s.approvalCmd}</code> {s.approvalIn} <b>{s.folder}</b></p>
      <p class="d">{s.approvalD}</p>
      <div class="btns">
        <button class="primary" onclick={() => run.approve()}>{s.allowOnce}</button>
        <button onclick={() => run.approve()}>{s.always}</button>
        <button class="ghost" onclick={() => run.deny()}>{s.deny}</button>
      </div>
    </div>
  {:else if run.stage === 7}
    <p class="saved" in:fly={{ y: 8, duration: 300 }}>
      <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M5 12.5l4.5 4.5L19 7.5" /></svg>{s.saved}
    </p>
  {/if}
{/if}

<style>
  .user { margin: 28px 0 14px auto; width: max-content; max-width: 80%; background: var(--user); padding: 10px 18px; border-radius: 14px; }
  .card { border: 1px solid color-mix(in srgb, var(--attn) 55%, var(--line)); background: var(--surface); border-radius: var(--r-md); padding: 16px 18px; animation: hold 1.8s ease-in-out infinite alternate; }
  @keyframes hold { from { box-shadow: 0 0 0 0 color-mix(in srgb, var(--attn) 0%, transparent); } to { box-shadow: 0 0 0 4px color-mix(in srgb, var(--attn) 18%, transparent); } }
  .head { display: flex; gap: 10px; align-items: center; margin-bottom: 6px; }
  .head svg { width: 18px; height: 18px; stroke: var(--attn); fill: none; stroke-width: 2; }
  code { font: 600 14px ui-monospace, Menlo, monospace; background: var(--raise); padding: 2px 6px; border-radius: 6px; }
  .d { color: var(--mute); font-size: 14.5px; margin: 4px 0 12px; }
  .btns { display: flex; gap: 8px; flex-wrap: wrap; }
  .btns button { padding: 7px 14px; border-radius: 6px; border: 1px solid var(--line); font-weight: 600; font-size: 14px; transition: background 150ms var(--ease-out), transform 120ms var(--ease-out); }
  .btns button:hover { background: var(--raise); }
  .btns button:active { transform: scale(0.97); }
  .btns .primary { background: var(--text); color: var(--bg); border-color: var(--text); }
  .btns .ghost { border-color: transparent; color: var(--mute); }
  .saved { display: flex; gap: 8px; align-items: center; color: var(--ok); font-weight: 600; }
  .saved svg { width: 18px; height: 18px; stroke: currentColor; fill: none; stroke-width: 2.4; stroke-dasharray: 24; stroke-dashoffset: 24; animation: tick 400ms var(--ease-out) forwards; }
  @keyframes tick { to { stroke-dashoffset: 0; } }
</style>

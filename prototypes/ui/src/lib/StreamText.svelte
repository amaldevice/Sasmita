<script lang="ts">
  import type { Token } from './run.svelte';

  let { tokens, shown, world, columns = false }: { tokens: Token[]; shown: number; world: string; columns?: boolean } = $props();
  let paras = $derived.by(() => {
    const out: { tok: Token; i: number }[][] = [];
    tokens.forEach((tok, i) => { if (i < shown) (out[tok.p] ??= []).push({ tok, i }); });
    return out;
  });
  let streaming = $derived(shown > 0 && shown < tokens.length);
</script>

<div class={['text', columns && 'cols']}>
  {#each paras as words, p (p)}
    <p>
      {#each words as { tok, i } (i)}
        {#if tok.bold}
          <strong class={['b', world === 'papan' && 'chalk']}>{tok.w}{#if world === 'papan'}<svg viewBox="0 0 60 9" preserveAspectRatio="none" aria-hidden="true"><path d="M1 6C12 3 24 7 36 4S54 5 59 3" pathLength="1" /></svg>{/if}</strong>{' '}
        {:else}
          <span class="w">{tok.w}</span>{' '}
        {/if}
      {/each}
      {#if streaming && p === paras.length - 1}<span class="caret" aria-hidden="true"></span>{/if}
    </p>
  {/each}
</div>

<style>
  .text { max-width: 68ch; }
  .cols { max-width: none; columns: 2; column-gap: 40px; }
  p { margin-bottom: 12px; }
  .w, .b { animation: word 380ms var(--ease-out) both; }
  @keyframes word { from { opacity: 0; filter: blur(3px); } }
  .b { font-weight: 700; }
  .chalk { position: relative; white-space: nowrap; }
  .chalk svg { position: absolute; left: -2px; bottom: -7px; width: calc(100% + 4px); height: 9px; overflow: visible; }
  .chalk path { stroke: var(--accent); stroke-width: 2.4; fill: none; stroke-linecap: round; stroke-dasharray: 1; stroke-dashoffset: 1; animation: draw 600ms var(--ease-out) 300ms forwards; }
  @keyframes draw { to { stroke-dashoffset: 0; } }
  .caret { display: inline-block; width: 8px; height: 1.05em; vertical-align: -2px; background: var(--accent); border-radius: 2px; animation: blink 900ms steps(2) infinite; }
  @keyframes blink { 50% { opacity: 0; } }
</style>

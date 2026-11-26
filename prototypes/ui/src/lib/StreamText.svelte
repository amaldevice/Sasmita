<script lang="ts">
  import type { Token } from './run.svelte';

  let { tokens, shown, columns = false }: { tokens: Token[]; shown: number; columns?: boolean } = $props();
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
          <strong class="b">{tok.w}</strong>{' '}
        {:else}
          <span class="w">{tok.w}</span>{' '}
        {/if}
      {/each}
      {#if streaming && p === paras.length - 1}<span class="caret" aria-hidden="true"></span>{/if}
    </p>
  {/each}
</div>

<style>
  .text { max-width: 70ch; }
  .cols p { break-inside: avoid; }
  .cols { max-width: none; columns: 2; column-gap: 40px; }
  p { margin-bottom: 12px; }
  .w, .b { animation: word 380ms var(--ease-out) both; }
  @keyframes word { from { opacity: 0; filter: blur(3px); } }
  .b { font-weight: 700; }
  .caret { display: inline-block; width: 8px; height: 1.05em; vertical-align: -2px; background: var(--accent); border-radius: 2px; animation: blink 900ms steps(2) infinite; }
  @keyframes blink { 50% { opacity: 0; } }
</style>

<script lang="ts">
  interface Props {
    label?: string;
    source?: string;
  }
  let { label = '', source = '' }: Props = $props();

  // Validated diagram text only: A[Label] --> B[Label]. No HTML is interpreted.
  const nodes = $derived.by(() =>
    source
      .split('-->')
      .map((part) => part.trim())
      .filter(Boolean)
      .map((part, index) => {
        const match = /^([A-Za-z0-9_-]+)\[(.+)\]$/.exec(part);
        return match ? { id: match[1], label: match[2] } : { id: `n${index}`, label: part };
      }),
  );
</script>

<div class="diagram" data-testid="ui-mermaid">
  {#if label}<div class="caption">{label}</div>{/if}
  <ol>
    {#each nodes as node, index (node.id + index)}
      <li data-testid="ui-mermaid-node">
        <span class="id num">{node.id}</span>
        <span class="text">{node.label}</span>
        {#if index < nodes.length - 1}<span class="arrow" aria-hidden="true">→</span>{/if}
      </li>
    {/each}
  </ol>
</div>

<style>
  .diagram { display: flex; flex-direction: column; gap: 6px; }
  .caption { color: var(--text); font-size: 13px; }
  ol { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; margin: 0; padding: 0; list-style: none; }
  li { display: inline-flex; align-items: center; gap: 6px; }
  .id { color: var(--mute); font-size: 11px; }
  .text { padding: 4px 9px; border: 1px solid var(--line); border-radius: 6px; color: var(--text); font-size: 13px; }
  .arrow { color: var(--mute); }
</style>

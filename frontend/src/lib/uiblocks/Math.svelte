<script lang="ts">
  interface Props {
    label?: string;
    source?: string;
  }
  let { label = '', source = '' }: Props = $props();
</script>

<div class="math" data-testid="ui-math">
  {#if label}<span class="label">{label}</span>{/if}
  <span class="formula">{source}</span>
</div>

<style>
  .math { display: flex; align-items: baseline; gap: 10px; font-size: 14px; }
  .label { color: var(--mute); font-size: 13px; }
  .formula { color: var(--text); font-family: var(--mono); font-size: 13px; }
</style>

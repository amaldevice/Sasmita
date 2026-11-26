<script lang="ts">
  import { displayValue, rowLabel } from '../uiblocks';

  interface Props {
    label?: string;
    metrics?: Record<string, unknown>[];
  }
  let { label = '', metrics = [] }: Props = $props();
</script>

<div class="kpi" data-testid="ui-kpi">
  {#if label}<div class="caption">{label}</div>{/if}
  <dl>
    {#each metrics as metric, index (index)}
      <div class="metric" data-testid="ui-kpi-metric">
        <dt>{rowLabel(metric)}</dt>
        <dd class="num">{displayValue(metric.value)}</dd>
      </div>
    {/each}
  </dl>
</div>

<style>
  .kpi { display: flex; flex-direction: column; gap: 6px; }
  .caption { color: var(--text); font-size: 13px; }
  dl { display: flex; flex-wrap: wrap; gap: 4px 18px; margin: 0; }
  .metric { display: inline-flex; align-items: baseline; gap: 8px; font-size: 13px; }
  dt { color: var(--mute); }
  dd { margin: 0; color: var(--text); }
</style>

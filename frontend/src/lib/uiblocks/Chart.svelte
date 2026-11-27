<script lang="ts">
  import { displayValue, rowLabel, rowValue } from '../uiblocks';
  import { i18n } from '../i18n.svelte';

  interface Props {
    label?: string;
    points?: Record<string, unknown>[];
  }
  let { label = '', points = [] }: Props = $props();
  const top = $derived(Math.max(1, ...points.map(rowValue)));
</script>

<div class="chart" data-testid="ui-chart">
  {#if label}<div class="caption">{label}</div>{/if}
  {#each points as point, index (index)}
    <div class="row" data-testid="ui-chart-point">
      <span class="name">{rowLabel(point)}</span>
      <span class="track"><span class="bar" style:width={`${Math.max(2, (rowValue(point) / top) * 100)}%`}></span></span>
      <span class="value num">{displayValue(rowValue(point))}</span>
    </div>
  {/each}
  {#if points.length === 0}<p class="empty">{i18n.t('noDataPoints')}</p>{/if}
</div>

<style>
  .chart { display: flex; flex-direction: column; gap: 6px; }
  .caption { color: var(--text); font-size: 13px; }
  .row { display: grid; grid-template-columns: 76px 1fr auto; align-items: center; gap: 10px; font-size: 13px; }
  .name { color: var(--mute); font-family: var(--mono); font-size: 11px; }
  .track { display: block; height: 8px; background: var(--raise); border-radius: 2px; }
  .bar { display: block; height: 100%; background: var(--flow); border-radius: 2px; }
  .value { color: var(--text); font-size: 12px; }
  .empty { color: var(--mute); font-size: 13px; }
</style>

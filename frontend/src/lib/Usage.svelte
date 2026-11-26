<script lang="ts">
  import { i18n } from './i18n.svelte';
  import { formatCost, formatCount, formatPercent, formatUsageLine, type UsageData } from './usage';

  let {
    usage,
    alwaysShow = false,
    open = false,
  }: { usage: UsageData | null; alwaysShow?: boolean; open?: boolean } = $props();
</script>

{#if usage}
  {#if alwaysShow}
    <div class="line num" data-testid="usage-line">{formatUsageLine(usage, i18n.locale)}</div>
  {/if}

  {#if open}
    <div class="popover" role="tooltip" data-testid="usage-popover">
      <div class="label">{i18n.t('usageDetails')}</div>
      <dl>
        <dt>{i18n.t('inputTokens')}</dt><dd class="num">{formatCount(usage.inputTokens, i18n.locale)}</dd>
        <dt>{i18n.t('outputTokens')}</dt><dd class="num">{formatCount(usage.outputTokens, i18n.locale)}</dd>
        <dt>{i18n.t('cacheHit')}</dt><dd class="num">{formatPercent(usage.hitRate, i18n.locale)}</dd>
        <dt>{i18n.t('cost')}</dt><dd class="num">{formatCost(usage, i18n.locale)}</dd>
      </dl>
      <div class="note">{usage.costEstimated ? i18n.t('appEstimate') : i18n.t('providerReported')}</div>
    </div>
  {/if}
{/if}

<style>
  .line {
    margin: 8px 0 0;
    color: var(--mute);
    font-size: 12px;
  }
  .popover {
    position: absolute;
    right: 16px;
    bottom: 12px;
    z-index: 2;
    width: max-content;
    max-width: 320px;
    padding: 12px 14px;
    border: 1px solid var(--line);
    border-radius: 10px;
    background: var(--surface);
    box-shadow: 0 8px 24px rgb(0 0 0 / 25%);
    pointer-events: none;
  }
  .label {
    margin-bottom: 8px;
    color: var(--mute);
    font-size: 11px;
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }
  dl {
    display: grid;
    grid-template-columns: auto auto;
    gap: 4px 20px;
    margin: 0;
    font-size: 13px;
  }
  dt { color: var(--mute); }
  dd { margin: 0; text-align: right; color: var(--text); }
  .note {
    margin: 10px 0 0;
    color: var(--mute);
    font-size: 12px;
  }
</style>

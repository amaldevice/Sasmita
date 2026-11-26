<script lang="ts">
  import { onMount } from 'svelte';
  import { i18n } from './i18n.svelte';
  import { createUsageIPC, formatCost, formatPercent, type UsageReport } from './usage';

  let report = $state<UsageReport | null>(null);
  let failed = $state(false);

  onMount(() => {
    void createUsageIPC()
      .getUsageReport()
      .then((loaded) => { report = loaded; })
      .catch(() => { failed = true; });
  });
</script>

<section class="diagnostics" data-testid="diagnostics">
  <h2>{i18n.t('diagnostics')}</h2>

  {#if failed}
    <p class="empty-note">{i18n.t('diagnosticsUnavailable')}</p>
  {/if}

  {#if report?.drop}
    <p class="warning" data-testid="hit-rate-drop">
      {i18n.t('hitRateDrop')} {report.drop.day}:
      {formatPercent(report.drop.from, i18n.locale)} → {formatPercent(report.drop.to, i18n.locale)}
    </p>
  {/if}

  <h3>{i18n.t('perModel')}</h3>
  <table data-testid="diagnostics-models">
    <thead>
      <tr>
        <th>{i18n.t('model')}</th>
        <th>{i18n.t('requests')}</th>
        <th>{i18n.t('inputTokens')}</th>
        <th>{i18n.t('outputTokens')}</th>
        <th>{i18n.t('cacheHit')}</th>
        <th>{i18n.t('cost')}</th>
      </tr>
    </thead>
    <tbody>
      {#each report?.models ?? [] as row (row.key)}
        <tr data-testid="model-row">
          <td>{row.key}</td>
          <td class="num">{row.requests}</td>
          <td class="num">{row.inputTokens}</td>
          <td class="num">{row.outputTokens}</td>
          <td class="num">{formatPercent(row.hitRate, i18n.locale)}</td>
          <td class="num">
            {formatCost(row.costUsd, i18n.locale)}
            {#if row.costEstimated}<span class="estimate">{i18n.t('estimate')}</span>{/if}
          </td>
        </tr>
      {/each}
    </tbody>
  </table>

  <h3>{i18n.t('perDay')}</h3>
  <table data-testid="diagnostics-days">
    <thead>
      <tr>
        <th>{i18n.t('day')}</th>
        <th>{i18n.t('requests')}</th>
        <th>{i18n.t('inputTokens')}</th>
        <th>{i18n.t('outputTokens')}</th>
        <th>{i18n.t('cacheHit')}</th>
        <th>{i18n.t('cost')}</th>
      </tr>
    </thead>
    <tbody>
      {#each report?.days ?? [] as row (row.key)}
        <tr data-testid="day-row">
          <td>{row.key}</td>
          <td class="num">{row.requests}</td>
          <td class="num">{row.inputTokens}</td>
          <td class="num">{row.outputTokens}</td>
          <td class="num">{formatPercent(row.hitRate, i18n.locale)}</td>
          <td class="num">{formatCost(row.costUsd, i18n.locale)}</td>
        </tr>
      {/each}
    </tbody>
  </table>
</section>

<style>
  .diagnostics { margin-top: 12px; }
  h2 { margin: 0 0 12px; font-size: 16px; font-weight: 600; }
  h3 { margin: 20px 0 8px; color: var(--mute); font-size: 13px; font-weight: 600; }
  .warning {
    margin: 0;
    padding: 10px 12px;
    border: 1px solid var(--attn);
    border-radius: 10px;
    color: var(--attn);
    font-size: 13px;
  }
  table { width: 100%; border-collapse: collapse; font-size: 13px; }
  th, td { padding: 8px 6px; border-bottom: 1px solid var(--line); text-align: right; }
  th:first-child, td:first-child { text-align: left; }
  th { color: var(--mute); font-weight: 500; }
  .num { font-family: var(--mono); font-variant-numeric: tabular-nums; }
  .estimate { margin-left: 6px; color: var(--mute); font-size: 11px; }
  .empty-note { color: var(--mute); }
</style>

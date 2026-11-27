<script lang="ts">
  import { displayValue } from '../uiblocks';
  import { i18n } from '../i18n.svelte';

  interface Props {
    label?: string;
    rows?: Record<string, unknown>[];
  }
  let { label = '', rows = [] }: Props = $props();
  const columns = $derived(rows.length > 0 ? Object.keys(rows[0]) : []);
</script>

<div class="table-wrap" data-testid="ui-table">
  {#if label}<div class="caption">{label}</div>{/if}
  <table>
    <thead>
      <tr>
        {#each columns as column (column)}<th scope="col">{column}</th>{/each}
      </tr>
    </thead>
    <tbody>
      {#each rows as row, index (index)}
        <tr>
          {#each columns as column (column)}<td class:num={typeof row[column] === 'number'}>{displayValue(row[column])}</td>{/each}
        </tr>
      {/each}
    </tbody>
  </table>
  {#if rows.length === 0}<p class="empty">{i18n.t('noDataRows')}</p>{/if}
</div>

<style>
  .table-wrap { display: flex; flex-direction: column; gap: 6px; }
  .caption { color: var(--text); font-size: 13px; }
  table { width: 100%; border-collapse: collapse; font-size: 13px; }
  th { padding: 5px 8px; border-bottom: 1px solid var(--line); color: var(--mute); text-align: left; font-weight: 500; }
  td { padding: 5px 8px; border-bottom: 1px solid var(--line); color: var(--text); }
  .num { font-family: var(--mono); font-size: 12px; }
  .empty { color: var(--mute); font-size: 13px; }
</style>

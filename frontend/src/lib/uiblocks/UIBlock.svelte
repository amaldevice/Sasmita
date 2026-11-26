<script lang="ts">
  import { onDestroy, untrack } from 'svelte';
  import type { UIData, UIBlock } from '../types';
  import { bindingKey, dataKey, displayValue, withState } from '../uiblocks';
  import Chart from './Chart.svelte';
  import Kpi from './Kpi.svelte';
  import Math from './Math.svelte';
  import Mermaid from './Mermaid.svelte';
  import Table from './Table.svelte';

  interface Props {
    block: UIBlock;
    getUIData: (ref: string, limit: number) => Promise<UIData>;
    onStateCommit: (blockId: string, patch: Record<string, unknown>) => void;
  }
  let { block, getUIData, onStateCommit }: Props = $props();

  // Named uiState: a variable called `state` would make `$state(...)` parse as
  // a store subscription in Svelte. untrack keeps the initial value synchronous
  // so a fast interaction cannot race with a deferred effect.
  let uiState = $state<Record<string, unknown>>(untrack(() => ({ ...(block.spec?.state ?? {}) })));
  let dataRows = $state<Record<string, unknown>[]>([]);
  let timer: ReturnType<typeof setTimeout> | undefined;
  let currentId = untrack(() => block.blockId);

  const chartComponent = $derived(block.spec?.components.find((c) => c.type === 'chart'));
  const threshold = $derived(Number(uiState.threshold ?? Number.MAX_SAFE_INTEGER));
  const resolution = $derived(String(uiState.resolution ?? ''));
  const showErrors = $derived(Boolean(uiState.showErrors));
  const chartPoints = $derived((chartComponent?.values ?? []).filter((point) => Number(point.value ?? 0) <= threshold));

  $effect(() => {
    if (block.blockId === currentId) return;
    currentId = block.blockId;
    uiState = { ...(block.spec?.state ?? {}) };
    dataRows = [];
  });

  $effect(() => {
    const spec = block.spec;
    if (!spec) return;
    let cancelled = false;
    for (const component of spec.components) {
      const key = dataKey(component.dataRef);
      const source = key ? spec.data?.[key]?.source : undefined;
      if (!source) continue;
      void getUIData(source, component.type === 'table' ? 200 : 5000)
        .then((data) => { if (!cancelled) dataRows = data.rows; })
        .catch(() => { if (!cancelled) dataRows = []; });
    }
    return () => { cancelled = true; };
  });

  onDestroy(() => { if (timer) clearTimeout(timer); });

  /** View-local update first, debounced persistence second. Never a model call. */
  function commit(bind: string | undefined, value: unknown): void {
    uiState = withState(uiState, bind, value);
    const key = bindingKey(bind);
    if (!key) return;
    if (timer) clearTimeout(timer);
    timer = setTimeout(() => {
      timer = undefined;
      onStateCommit(block.blockId, { [key]: value });
    }, 300);
  }

  function valueOf(bind: string | undefined): unknown {
    const key = bindingKey(bind);
    return key ? uiState[key] : undefined;
  }
</script>

{#if !block.spec}
  <article class="block fallback" data-testid="ui-block-fallback">
    <p>{block.reason || 'UI block tidak tersedia.'}</p>
    <p class="hint">Jawaban teks tetap disimpan.</p>
  </article>
{:else}
  <article class="block" data-testid="ui-block" data-block-id={block.blockId} aria-label={block.spec.title || block.spec.summary}>
    {#if block.spec.title}<h2>{block.spec.title}</h2>{/if}
    <p class="summary">{block.spec.summary}</p>

    <div class="controls">
      {#each block.spec.components.filter((c) => ['tabs', 'slider', 'toggle'].includes(c.type)) as component (component.id)}
        {#if component.type === 'tabs'}
          <div class="control" data-testid="ui-control-tabs">
            <span class="label">{component.label}</span>
            <div class="segmented" role="group" aria-label={component.label}>
              {#each component.options ?? [] as option (option)}
                <button
                  type="button"
                  class:active={String(valueOf(component.bind) ?? '') === option}
                  data-testid={`ui-tab-${option}`}
                  onclick={() => commit(component.bind, option)}
                >{option}</button>
              {/each}
            </div>
          </div>
        {:else if component.type === 'slider'}
          <label class="control" data-testid="ui-control-slider">
            <span class="label">{component.label} <span class="num">{displayValue(valueOf(component.bind))}</span></span>
            <input
              type="range"
              data-testid="ui-slider"
              min={component.min ?? 0}
              max={component.max ?? 100}
              step={component.step ?? 1}
              value={Number(valueOf(component.bind) ?? component.min ?? 0)}
              oninput={(event) => commit(component.bind, Number(event.currentTarget.value))}
            />
          </label>
        {:else if component.type === 'toggle'}
          <label class="control" data-testid="ui-control-toggle">
            <input
              type="checkbox"
              data-testid="ui-toggle"
              checked={Boolean(valueOf(component.bind))}
              onchange={(event) => commit(component.bind, event.currentTarget.checked)}
            />
            <span class="label">{component.label}</span>
          </label>
        {/if}
      {/each}
    </div>

    <div class="visuals">
      {#each block.spec.components.filter((c) => !['tabs', 'slider', 'toggle'].includes(c.type)) as component (component.id)}
        {#if component.type === 'chart'}
          <Chart label={`${component.label ?? ''}${resolution ? ` (${resolution})` : ''}`} points={chartPoints} />
        {:else if component.type === 'table'}
          <Table label={component.label} rows={component.dataRef ? dataRows : (component.values ?? [])} />
        {:else if component.type === 'mermaid'}
          <Mermaid label={component.label} source={component.source} />
        {:else if component.type === 'kpi'}
          <Kpi label={component.label} metrics={component.values ?? []} />
        {:else if component.type === 'math'}
          <Math label={component.label} source={component.source} />
        {/if}
      {/each}
    </div>

    {#if showErrors}<p class="note" data-testid="ui-local-note">Menampilkan galat secara lokal.</p>{/if}
  </article>
{/if}

<style>
  .block { max-width: 760px; margin: 0 0 20px; padding: 16px; border: 1px solid var(--line); border-radius: 10px; background: var(--surface); }
  h2 { margin: 0 0 4px; font-size: 17px; font-weight: 600; }
  .summary { margin: 0 0 14px; color: var(--mute); font-size: 13px; line-height: 1.5; }
  .controls { display: flex; flex-wrap: wrap; align-items: center; gap: 16px; margin-bottom: 14px; }
  .control { display: inline-flex; align-items: center; gap: 8px; font-size: 13px; }
  .label { color: var(--mute); }
  .num { color: var(--text); font-family: var(--mono); font-size: 11px; }
  .segmented { display: inline-flex; border: 1px solid var(--line); border-radius: 6px; overflow: hidden; }
  .segmented button { padding: 5px 10px; color: var(--mute); font-size: 12px; }
  .segmented button.active { background: var(--raise); color: var(--text); }
  input[type='range'] { width: 150px; accent-color: var(--accent); }
  input[type='checkbox'] { accent-color: var(--accent); }
  .visuals { display: flex; flex-direction: column; gap: 14px; }
  .note, .hint { margin: 10px 0 0; color: var(--mute); font-size: 12px; }
  .fallback { color: var(--attn); }
</style>

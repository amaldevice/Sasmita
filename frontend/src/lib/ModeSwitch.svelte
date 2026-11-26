<script lang="ts">
  import type { ModelInfo } from './connections';
  import { i18n } from './i18n.svelte';
  import { modeLabel, type Mode, type SwitchEstimate } from './modes';

  type Choice = 'mode' | 'model';

  let {
    modes,
    models,
    modeId,
    modelId,
    estimate,
    onselect,
  }: {
    modes: Mode[];
    models: ModelInfo[];
    modeId: string;
    modelId: string;
    estimate: (kind: Choice, value: string) => Promise<SwitchEstimate>;
    onselect: (kind: Choice, value: string) => void;
  } = $props();

  let open = $state<Choice | null>(null);
  let pending = $state<{ kind: Choice; value: string; tokens: number } | null>(null);
  let busy = $state(false);

  const activeMode = $derived(modes.find((mode) => mode.id === modeId));

  function toggle(kind: Choice) {
    pending = null;
    open = open === kind ? null : kind;
  }

  async function choose(kind: Choice, value: string) {
    if (busy) return;
    const current = kind === 'mode' ? modeId : modelId;
    if (value === current) {
      open = null;
      return;
    }
    busy = true;
    try {
      const result = await estimate(kind, value);
      open = null;
      if (result.warn) {
        pending = { kind, value, tokens: result.estimatedTokens };
      } else {
        onselect(kind, value);
      }
    } finally {
      busy = false;
    }
  }

  function confirm() {
    if (!pending) return;
    onselect(pending.kind, pending.value);
    pending = null;
  }
</script>

<div class="chips">
  <div class="chip-wrap">
    <button
      type="button"
      class="chip"
      data-testid="mode-chip"
      aria-haspopup="listbox"
      aria-expanded={open === 'mode'}
      onclick={() => toggle('mode')}
    >
      <span class="key">{i18n.t('mode')}</span>{modeLabel(activeMode, i18n.locale)}
    </button>
    {#if open === 'mode'}
      <div class="menu" role="listbox" aria-label={i18n.t('mode')} data-testid="mode-menu">
        {#each modes as mode (mode.id)}
          <div data-testid="mode-option">
            <button
              type="button"
              role="option"
              aria-selected={mode.id === modeId}
              class:active={mode.id === modeId}
              data-testid={`mode-option-${mode.id}`}
              disabled={!mode.available}
              onclick={() => void choose('mode', mode.id)}
            >
              <span>{modeLabel(mode, i18n.locale)}</span>
              {#if !mode.available}<small>{i18n.t('comingLater')}</small>{/if}
            </button>
          </div>
        {/each}
      </div>
    {/if}
  </div>

  <div class="chip-wrap">
    <button
      type="button"
      class="chip"
      data-testid="model-chip"
      aria-haspopup="listbox"
      aria-expanded={open === 'model'}
      onclick={() => toggle('model')}
    >
      <span class="key">{i18n.t('model')}</span>{modelId || '—'}
    </button>
    {#if open === 'model'}
      <div class="menu" role="listbox" aria-label={i18n.t('model')} data-testid="model-menu">
        {#each models as model (model.id)}
          <div data-testid="model-option">
            <button
              type="button"
              role="option"
              aria-selected={model.id === modelId}
              class:active={model.id === modelId}
              data-testid={`model-option-${model.id}`}
              onclick={() => void choose('model', model.id)}
            >
              <span>{model.id}</span>
              {#if model.capabilities.contextWindow !== null}
                <small class="num">{i18n.formatNumber(model.capabilities.contextWindow)}</small>
              {/if}
            </button>
          </div>
        {/each}
      </div>
    {/if}
  </div>

  {#if pending}
    <div class="popover" role="dialog" aria-label={i18n.t('rereadWarning')} data-testid="reread-warning">
      <p>{i18n.t('rereadWarning')} ≈ {i18n.formatNumber(pending.tokens)} {i18n.t('tokens')}</p>
      <div class="popover-actions">
        <button type="button" data-testid="reread-cancel" onclick={() => (pending = null)}>{i18n.t('cancel')}</button>
        <button type="button" class="primary" data-testid="reread-confirm" onclick={confirm}>{i18n.t('continue')}</button>
      </div>
    </div>
  {/if}
</div>

<style>
  .chips { position: relative; display: flex; align-items: center; gap: 8px; margin-bottom: 10px; }
  .chip-wrap { position: relative; }
  .chip {
    display: inline-flex; align-items: baseline; gap: 7px; padding: 6px 10px;
    border: 1px solid var(--line); border-radius: 6px; background: var(--surface);
    color: var(--text); font-size: 13px;
  }
  .chip:hover, .chip[aria-expanded='true'] { background: var(--raise); }
  .key { color: var(--mute); }
  .menu {
    position: absolute; bottom: calc(100% + 6px); left: 0; z-index: 3; min-width: 200px;
    padding: 5px; border: 1px solid var(--line); border-radius: 10px; background: var(--surface);
    box-shadow: 0 12px 28px rgba(0, 0, 0, 0.28);
  }
  .menu button {
    display: flex; width: 100%; align-items: center; justify-content: space-between; gap: 12px;
    padding: 8px 10px; border-radius: 6px; color: var(--text); text-align: left;
  }
  .menu button:hover:not(:disabled), .menu button.active { background: var(--raise); }
  .menu button:disabled { color: var(--mute); cursor: not-allowed; }
  .menu small { color: var(--mute); font-size: 11px; }
  .popover {
    position: absolute; bottom: calc(100% + 6px); left: 50%; z-index: 4;
    transform: translateX(-50%); width: max-content; max-width: 320px;
    padding: 12px 14px; border: 1px solid var(--attn); border-radius: 10px;
    background: var(--surface); color: var(--text);
  }
  .popover p { margin: 0 0 10px; font-size: 13px; }
  .popover-actions { display: flex; justify-content: flex-end; gap: 8px; }
  .popover-actions button { padding: 6px 10px; border-radius: 6px; background: var(--raise); color: var(--text); }
  .popover-actions .primary { background: var(--text); color: var(--ground); font-weight: 600; }
</style>

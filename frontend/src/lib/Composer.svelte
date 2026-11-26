<script lang="ts">
  import type { ModelInfo } from './connections';
  import { i18n } from './i18n.svelte';
  import ModeSwitch from './ModeSwitch.svelte';
  import type { Mode, SwitchEstimate } from './modes';

  type Choice = 'mode' | 'model';

  let {
    value = $bindable(''),
    onsubmit,
    modes = [],
    models = [],
    modeId = 'chat',
    modelId = '',
    estimate = async () => ({ estimatedTokens: 0, warn: false, estimated: true }),
    onselect = () => {},
  }: {
    value: string;
    onsubmit: (text: string) => void;
    modes?: Mode[];
    models?: ModelInfo[];
    modeId?: string;
    modelId?: string;
    estimate?: (kind: Choice, value: string) => Promise<SwitchEstimate>;
    onselect?: (kind: Choice, value: string) => void;
  } = $props();

  function submit(event: SubmitEvent) {
    event.preventDefault();
    const text = value.trim();
    if (!text) return;
    onsubmit(text);
    value = '';
  }
</script>

<form class="composer" onsubmit={submit}>
  <div class="composer-body">
    <ModeSwitch {modes} {models} {modeId} {modelId} {estimate} {onselect} />
    <textarea
      data-testid="composer-input"
      bind:value
      rows="2"
      placeholder={i18n.t('askAnything')}
      aria-label={i18n.t('message')}
    ></textarea>
  </div>
  <button data-testid="send-button" type="submit">{i18n.t('send')}</button>
</form>

<style>
  .composer {
    display: flex;
    align-items: flex-end;
    gap: 10px;
    padding: 12px;
    border: 1px solid var(--line);
    border-radius: 16px;
    background: var(--surface);
  }
  .composer-body { flex: 1; min-width: 0; }
  textarea {
    display: block;
    width: 100%;
    resize: none;
    border: 0;
    background: transparent;
    color: var(--text);
    font: inherit;
    line-height: 1.6;
    outline: none;
  }
  button {
    padding: 8px 16px;
    border-radius: 6px;
    background: var(--text);
    color: var(--ground);
    font-weight: 600;
  }
  button:hover { background: var(--accent); }
</style>

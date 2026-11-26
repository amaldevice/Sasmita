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
    attachments = [],
    onattach = () => {},
    onremoveattachment = () => {},
  }: {
    value: string;
    onsubmit: (text: string) => void;
    modes?: Mode[];
    models?: ModelInfo[];
    modeId?: string;
    modelId?: string;
    estimate?: (kind: Choice, value: string) => Promise<SwitchEstimate>;
    onselect?: (kind: Choice, value: string) => void;
    attachments?: string[];
    onattach?: () => void | Promise<void>;
    onremoveattachment?: (path: string) => void;
  } = $props();

  function basename(path: string): string {
    return path.split(/[\\/]/).pop() ?? path;
  }

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
  <div class="composer-actions">
    <button data-testid="attach-button" type="button" onclick={() => void onattach()}>{i18n.t('attach')}</button>
    <button data-testid="send-button" type="submit">{i18n.t('send')}</button>
  </div>
</form>
{#if attachments.length > 0}
  <div class="attachments" data-testid="attachment-list">
    {#each attachments as path (path)}
      <button data-testid="attachment-chip" title={path} onclick={() => onremoveattachment(path)}>{basename(path)} ×</button>
    {/each}
  </div>
{/if}

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
  .composer-actions { display: flex; flex-direction: column; gap: 6px; align-items: stretch; }
  .composer-actions button[data-testid='attach-button'] {
    padding: 7px 12px; border: 1px solid var(--line); border-radius: 6px;
    background: var(--surface); color: var(--mute); font-weight: 400;
  }
  .attachments { display: flex; flex-wrap: wrap; gap: 6px; margin: 8px 24px -12px; }
  .attachments button {
    padding: 5px 9px; border: 1px solid var(--line); border-radius: 6px;
    background: var(--raise); color: var(--mute); font-size: 12px;
  }
</style>

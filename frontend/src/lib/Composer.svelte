<script lang="ts">
  import { i18n } from './i18n.svelte';

  let { value = $bindable(''), onsubmit }: { value: string; onsubmit: (text: string) => void } = $props();

  function submit(event: SubmitEvent) {
    event.preventDefault();
    const text = value.trim();
    if (!text) return;
    onsubmit(text);
    value = '';
  }
</script>

<form class="composer" onsubmit={submit}>
  <textarea
    data-testid="composer-input"
    bind:value
    rows="2"
    placeholder={i18n.t('askAnything')}
    aria-label={i18n.t('message')}
  ></textarea>
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
  textarea {
    flex: 1;
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

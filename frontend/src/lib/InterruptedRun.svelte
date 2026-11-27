<script lang="ts">
  import { i18n } from './i18n.svelte';
  import type { RunRecord } from './types';

  interface Props {
    run: RunRecord;
    onContinue: () => void;
  }

  let { run, onContinue }: Props = $props();
</script>

<div class="interrupted" data-testid="interrupted-run">
  <div>
    <strong>{i18n.t('interruptedRunTitle')}</strong>
    <span>{i18n.t('interruptedRunBody')}</span>
  </div>
  <button type="button" data-testid="continue-run" onclick={onContinue}>{i18n.t('continue')}</button>
</div>

<style>
  .interrupted {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    margin: 0 0 20px;
    padding: 12px 16px;
    border: 1px solid var(--attn);
    border-radius: 10px;
    background: var(--raise);
    font-size: 14px;
  }
  span { display: block; margin-top: 4px; color: var(--mute); }
  button { padding: 7px 14px; border: 1px solid var(--flow); border-radius: 6px; color: var(--text); }
  button:hover { background: var(--surface); }
</style>

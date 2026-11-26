<script lang="ts">
  import { activityLabel } from './activity';

  interface Props {
    status: string;
    cancelling?: boolean;
    onCancel: () => void;
  }

  let { status, cancelling = false, onCancel }: Props = $props();
</script>

<div class="activity" data-testid="run-status">
  <span>{activityLabel(status)}</span>
  {#if !cancelling}
    <button type="button" data-testid="cancel-run" onclick={onCancel}>Hentikan</button>
  {/if}
</div>

<style>
  .activity { display: flex; align-items: center; gap: 12px; color: var(--mute); font-size: 14px; }
  button { padding: 5px 10px; border: 1px solid var(--line); border-radius: 6px; color: var(--text); font-size: 12px; }
  button:hover { background: var(--raise); }
</style>

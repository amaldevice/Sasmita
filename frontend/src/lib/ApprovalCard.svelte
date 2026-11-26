<script lang="ts">
  import { i18n } from './i18n.svelte';
  import type { Approval, ApprovalDecision } from './types';

  let {
    approval,
    onresolve,
  }: {
    approval: Approval;
    onresolve: (decision: ApprovalDecision) => void;
  } = $props();
</script>

<section
  class="approval"
  data-testid="approval-card"
  data-pulsing="true"
  aria-label={i18n.t('approvalNeeded')}
>
  <div class="head">
    <span class="signal" aria-hidden="true"></span>
    <strong data-testid="approval-action">{approval.action}</strong>
  </div>
  <dl>
    <dt>{i18n.t('resource')}</dt>
    <dd data-testid="approval-resource">{approval.resource}</dd>
    <dt>{i18n.t('impact')}</dt>
    <dd data-testid="approval-impact">{approval.impact}</dd>
  </dl>
  <div class="actions">
    <button class="primary" data-testid="approve-once" onclick={() => onresolve('allow_once')}>
      {i18n.t('allowOnce')}
    </button>
    <button data-testid="approve-always" onclick={() => onresolve('always')}>
      {i18n.t('allowAlways')}
    </button>
    <button data-testid="deny-approval" onclick={() => onresolve('deny')}>
      {i18n.t('deny')}
    </button>
  </div>
</section>

<style>
  .approval {
    max-width: 760px;
    margin: 0 0 20px;
    padding: 14px 16px;
    border: 1px solid var(--attn);
    border-radius: 10px;
    background: var(--surface);
    animation: approval-pulse 1.8s ease-in-out infinite;
  }
  .head { display: flex; align-items: center; gap: 8px; margin-bottom: 8px; }
  .signal { width: 8px; height: 8px; flex: none; border-radius: 50%; background: var(--attn); }
  dl { display: grid; grid-template-columns: auto 1fr; gap: 4px 12px; margin: 0; font-size: 14px; }
  dt { color: var(--mute); }
  dd { margin: 0; }
  .actions { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 12px; }
  button { padding: 7px 12px; border: 1px solid var(--line); border-radius: 6px; color: var(--text); font-size: 13px; }
  button:hover { background: var(--raise); }
  button.primary { border-color: var(--attn); background: var(--attn); color: var(--bg); }
  @keyframes approval-pulse {
    0%, 100% { border-color: var(--attn); box-shadow: 0 0 0 0 transparent; }
    50% { border-color: var(--attn); box-shadow: 0 0 0 3px color-mix(in srgb, var(--attn) 22%, transparent); }
  }
  @media (prefers-reduced-motion: reduce) {
    .approval { animation: none; }
  }
</style>

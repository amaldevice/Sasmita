<script lang="ts">
  import { activityLabel, isActiveStatus } from './activity';
  import type { ToolStep } from './types';

  interface Props {
    status: string;
    cancelling?: boolean;
    onCancel: () => void;
    steps?: ToolStep[];
    elapsedSeconds?: number;
  }

  let { status, cancelling = false, onCancel, steps = [], elapsedSeconds = 0 }: Props = $props();
  let expanded = $state(false);
  const working = $derived(isActiveStatus(status));

  function phaseLabel(phase: ToolStep['phase']): string {
    switch (phase) {
      case 'proposed':
        return 'usulan';
      case 'executed':
        return 'dijalankan';
      case 'artifact':
        return 'artefak';
      case 'denied':
        return 'ditolak';
      default:
        return 'gagal';
    }
  }
</script>

<div class="activity" data-testid="run-status">
  <div class="line" data-testid="activity-line">
    {#if working}
      <span class="spinner" data-testid="activity-spinner" aria-hidden="true"></span>
    {/if}
    <span>{working ? `Bekerja ${elapsedSeconds} dtk` : activityLabel(status)}</span>
    {#if steps.length > 0}
      <button
        class="disclose"
        class:open={expanded}
        data-testid="timeline-toggle"
        aria-expanded={expanded}
        aria-label={expanded ? 'Tutup linimasa' : 'Buka linimasa'}
        onclick={() => (expanded = !expanded)}
      >›</button>
    {/if}
  </div>
  {#if !cancelling}
    <button type="button" data-testid="cancel-run" onclick={onCancel}>Hentikan</button>
  {/if}
</div>

{#if expanded && steps.length > 0}
  <ol class="timeline" data-testid="tool-timeline">
    {#each steps as step, index (step.callId + '-' + step.phase + '-' + index)}
      <li class="step" data-testid="tool-step" data-phase={step.phase}>
        <span class="marker" class:proposed={step.phase === 'proposed'} aria-hidden="true"></span>
        <span class="chip" class:proposed={step.phase === 'proposed'}>{step.name}</span>
        <span class="phase">{phaseLabel(step.phase)}</span>
        {#if step.output}<span class="detail">{step.output}</span>{/if}
        {#if step.summary}<span class="detail">{step.summary}</span>{/if}
        {#if step.artifactRef}
          <span class="ref" data-testid="tool-artifact-ref">{step.artifactRef}</span>
        {/if}
      </li>
    {/each}
  </ol>
{/if}

<style>
  .activity { display: flex; align-items: center; justify-content: space-between; gap: 12px; color: var(--mute); font-size: 14px; }
  .line { display: inline-flex; align-items: center; gap: 8px; }
  .spinner { width: 12px; height: 12px; border: 2px solid var(--line); border-top-color: var(--flow); border-radius: 50%; animation: spin 0.9s linear infinite; }
  @keyframes spin { to { transform: rotate(360deg); } }
  .disclose { color: var(--mute); font-size: 18px; line-height: 1; padding: 0 4px; }
  .disclose.open { transform: rotate(90deg); }
  button:not(.disclose) { padding: 5px 10px; border: 1px solid var(--line); border-radius: 6px; color: var(--text); font-size: 12px; }
  button:not(.disclose):hover { background: var(--raise); }
  .timeline { margin: 8px 0 0; padding: 0 0 0 12px; list-style: none; border-left: 1px solid var(--line); }
  .step { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; padding: 5px 0; font-size: 13px; }
  .marker { width: 6px; height: 6px; margin-left: -15px; border-radius: 50%; background: var(--flow); }
  .marker.proposed { background: transparent; border: 1px dashed var(--mute); }
  .chip { padding: 3px 7px; border: 1px solid var(--flow); border-radius: 6px; color: var(--text); font-family: var(--mono); font-size: 11px; }
  .chip.proposed { border-style: dashed; border-color: var(--mute); color: var(--mute); }
  .phase { color: var(--mute); font-size: 12px; }
  .detail { color: var(--text); font-family: var(--mono); font-size: 11px; }
  .ref { color: var(--ok); font-family: var(--mono); font-size: 11px; }
  @media (prefers-reduced-motion: reduce) {
    .spinner { animation: none; border-top-color: var(--flow); }
    .disclose.open { transform: none; }
  }
</style>

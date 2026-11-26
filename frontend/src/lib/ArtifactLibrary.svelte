<script lang="ts">
  import ArtifactFrame from './ArtifactFrame.svelte';
  import { pendingArtifact, type ArtifactRecord } from './artifacts';

  interface Props {
    artifacts: ArtifactRecord[];
    source?: string;
    /** Non-empty while a model-written source is still streaming in. */
    pendingSource?: string;
    data?: Record<string, unknown>;
    onSuggestPrompt?: (prompt: string) => void;
  }
  let { artifacts, source = '', pendingSource = '', data = {}, onSuggestPrompt }: Props = $props();

  let chosen = $state('');
  let selected = $derived(artifacts.find((a) => a.id === chosen) ?? artifacts[0] ?? null);
</script>

<section class="library" data-testid="artifacts-library">
  <h2>Sandboxed artifacts</h2>
  <div class="list">
    {#each artifacts as artifact (artifact.id)}
      <button
        class="row"
        class:active={selected?.id === artifact.id}
        data-testid="artifact-row"
        onclick={() => (chosen = artifact.id)}
      >
        <span class="title">{artifact.title}</span>
        <span class="num" data-testid="artifact-ref">{artifact.ref}</span>
        <span class="num" data-testid="artifact-versions">{artifact.versions.length} versi / versions</span>
        <span class="num" data-testid="artifact-runtime">{artifact.runtimeVersion}</span>
      </button>
    {/each}
    {#if artifacts.length === 0}
      <p class="empty-note" data-testid="artifacts-empty">Belum ada artifact / No artifacts yet</p>
    {/if}
  </div>

  {#if selected}
    <div class="detail">
      <h3>{selected.title}</h3>
      <ol class="num versions" data-testid="artifact-version-list">
        {#each selected.versions as version (version.version)}
          <li>v{version.version} · {version.sourceKind} · {version.sizeBytes} B · {version.sha256}</li>
        {/each}
      </ol>
      <ArtifactFrame artifact={selected} {source} {data} {onSuggestPrompt} />
    </div>
  {/if}

  {#if pendingSource}
    <div class="detail pending" data-testid="artifact-streaming">
      <h3>Sedang ditulis / Being written</h3>
      <ArtifactFrame artifact={pendingArtifact} source={pendingSource} {data} {onSuggestPrompt} />
    </div>
  {/if}
</section>

<style>
  .library { display: grid; grid-template-columns: minmax(220px, 300px) 1fr; gap: 20px; width: 100%; max-width: 1040px; }
  h2 { grid-column: 1 / -1; margin: 0 0 4px; font-size: 18px; font-weight: 600; }
  h3 { margin: 0 0 10px; font-size: 15px; font-weight: 600; }
  .list { display: flex; flex-direction: column; gap: 6px; }
  .row { display: flex; flex-direction: column; gap: 4px; padding: 10px 12px; border: 1px solid var(--line); border-radius: 10px; background: var(--surface); text-align: left; }
  .row.active { border-color: var(--flow); background: var(--raise); }
  .row .title { color: var(--text); font-weight: 600; }
  .num { font-family: var(--mono); font-size: 11px; color: var(--mute); }
  .versions { margin: 0 0 14px; padding-left: 20px; color: var(--mute); line-height: 1.8; }
  .empty-note { color: var(--mute); font-size: 13px; }
  @media (max-width: 760px) { .library { grid-template-columns: 1fr; } }
</style>

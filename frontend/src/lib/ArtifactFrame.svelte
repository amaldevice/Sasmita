<script lang="ts">
  import { onMount } from 'svelte';
  import {
    ArtifactBridge,
    bridgeLog,
    resetBridgeLog,
    type ArtifactBridgeOptions,
  } from './ArtifactBridge';
  import type { ArtifactRecord } from './artifacts';

  interface Props {
    artifact: ArtifactRecord;
    /** Model-written source shown while it streams, before it is stored. */
    source?: string;
    data?: Record<string, unknown>;
    onSuggestPrompt?: (prompt: string) => void;
  }
  let { artifact, source = '', data = {}, onSuggestPrompt }: Props = $props();

  let frameEl = $state<HTMLIFrameElement | null>(null);
  let width = $state(0);
  let height = $state(0);
  let error = $state('');
  let savedBytes = $state(0);

  // A froze_view artifact is never auto-run; an unstored artifact has no src.
  let storedOverride = $state('');
  let storedRef = $derived(storedOverride || (artifact.status === 'ok' ? artifact.ref : ''));
  let versionCount = $derived(artifact.versions.length);

  onMount(() => {
    resetBridgeLog();
    const options: ArtifactBridgeOptions = {
      frame: frameEl as HTMLIFrameElement,
      data,
      storage: localStorage,
      stateKey: `artifact.state.${artifact.id}`,
      onResize: (w, h) => { width = w; height = h; },
      onError: (message) => { error = message; },
      onSaveState: (bytes) => { savedBytes = bytes; },
      onSuggestPrompt: (prompt) => onSuggestPrompt?.(prompt),
    };
    const bridge = new ArtifactBridge(options);
    bridge.start();
    // Seam-2 harness only: the fake shell exposes the decisions for assertions.
    (window as unknown as { __artifactBridgeLog: typeof bridgeLog }).__artifactBridgeLog = bridgeLog;
    return () => bridge.stop();
  });
</script>

<section class="artifact" data-testid="artifact-frame">
  <header class="artifact-bar">
    <span data-testid="artifact-status">{artifact.status === 'ok' ? 'berjalan' : 'dibekukan'}</span>
    <span class="num" data-testid="artifact-version">v{versionCount}</span>
    {#if width > 0 && height > 0}
      <span class="num" data-testid="artifact-size">{width}×{height}</span>
    {/if}
    {#if savedBytes > 0}
      <span class="num" data-testid="artifact-saved">{savedBytes} B</span>
    {/if}
  </header>

  {#if !storedRef}
    <pre class="code" data-testid="artifact-code">{source}</pre>
    <p class="note" data-testid="artifact-pending">Kode disimpan dulu sebelum dijalankan — stored before running</p>
    <button class="store" data-testid="artifact-store" onclick={() => (storedOverride = 'artifact://' + artifact.id)}>
      Simpan & jalankan / Store & run
    </button>
  {/if}

  <!-- sandbox="allow-scripts" only: no allow-same-origin, never srcdoc. -->
  <iframe
    bind:this={frameEl}
    data-testid="artifact-iframe"
    title="Sandboxed artifact {artifact.id}"
    sandbox="allow-scripts"
    src={storedRef ? `${storedRef}/` : undefined}
  ></iframe>

  {#if error}
    <p class="error" data-testid="artifact-error">{error}</p>
  {/if}
</section>

<style>
  .artifact { display: flex; flex-direction: column; gap: 8px; min-height: 220px; }
  .artifact-bar { display: flex; align-items: center; gap: 12px; color: var(--mute); font-size: 12px; }
  .artifact-bar span:first-child { color: var(--ok); }
  .num { font-family: var(--mono); }
  iframe { flex: 1; width: 100%; min-height: 180px; border: 1px solid var(--line); border-radius: 8px; background: var(--surface); }
  .code { margin: 0; padding: 12px; max-height: 160px; overflow: auto; border: 1px solid var(--line); border-radius: 8px; background: var(--surface); color: var(--text); font-family: var(--mono); font-size: 12px; white-space: pre-wrap; }
  .note { margin: 0; color: var(--mute); font-size: 12px; }
  .store { align-self: flex-start; padding: 6px 10px; border: 1px solid var(--line); border-radius: 6px; color: var(--text); font-size: 12px; }
  .error { margin: 0; color: var(--attn); font-size: 12px; }
</style>

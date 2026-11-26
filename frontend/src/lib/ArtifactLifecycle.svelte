<script lang="ts">
  import { onMount } from 'svelte';
  import { bind, isMyGo } from 'mygo-runtime';
  import { loadArtifactState, saveArtifactState } from './ArtifactBridge';
  import {
    allocate, initial, lifecycleFixtures, reduce,
    type LifecycleAction, type LifecycleBlock,
  } from './artifactLifecycle';
  import { i18n } from './i18n.svelte';

  const reportLive = bind<[string[]], void>(
    'shellService.ReportLiveArtifacts', (ids: string[]) => [ids],
  );
  const openUntrusted = bind<[string], void>(
    'shellService.OpenArtifactWindow', (uri: string) => [uri],
  );

  // The library's records are the saved artifacts; this surface adds the
  // lifecycle state the renderer needs (visibility, slot claims, freezes).
  let flow = $state(allocate(initial(lifecycleFixtures())));
  let restored = $state<Record<string, boolean>>({});
  let showCode = $state<Record<string, boolean>>({});

  const blocks = $derived(flow.blocks);
  const liveIds = $derived(blocks.filter((b) => b.view === 'live').map((b) => b.id));

  // The Go watchdog blames the first live artifact when the main thread stops
  // answering. The trusted renderer is the only source of that list.
  $effect(() => {
    if (isMyGo()) void reportLive(liveIds);
  });

  function dispatch(action: LifecycleAction): void {
    flow = reduce(flow, action);
  }

  function stateKey(id: string): string {
    return `artifact.state.${id}`;
  }

  function offscreen(block: LifecycleBlock): void {
    saveArtifactState(stateKey(block.id), { version: block.version, savedAt: Date.now() });
    dispatch({ type: 'SCROLL_AWAY', id: block.id });
  }

  function run(block: LifecycleBlock): void {
    dispatch({ type: 'USER_RUN', id: block.id });
    dispatch({ type: 'SCROLL_INTO_VIEW', id: block.id });
    if (loadArtifactState(stateKey(block.id)) !== null) {
      restored = { ...restored, [block.id]: true };
    }
  }

  function openWindow(block: LifecycleBlock): void {
    dispatch({ type: 'OPEN_WINDOW', id: block.id });
    if (isMyGo()) void openUntrusted(block.uri);
    // Fake-IPC seam records the same decision: an Untrusted window, not a tab.
    const hook = (window as unknown as Record<string, (uri: string, untrusted: boolean) => void>).__sasmitaOpenUntrusted;
    hook?.(block.uri, true);
  }

  onMount(() => {
    // The Go watchdog reloads the page and invokes this hook with the offender
    // it marked froze_view in SQLite. The replay fixture uses the same hook.
    const previous = (window as unknown as Record<string, unknown>).__sasmitaWatchdog;
    (window as unknown as Record<string, unknown>).__sasmitaWatchdog = (id: string) => {
      dispatch({ type: 'WATCHDOG_FREEZE', id });
      const mark = (window as unknown as Record<string, (id: string) => void>).__sasmitaMarkFroze;
      mark?.(id);
    };
    return () => {
      (window as unknown as Record<string, unknown>).__sasmitaWatchdog = previous;
    };
  });
</script>

<section class="lifecycle" data-testid="artifact-lifecycle">
  <div class="bar">
    <h2>{i18n.t('liveArtifacts')}</h2>
    <button data-testid="lifecycle-reopen" onclick={() => dispatch({ type: 'RELOAD_CONVERSATION' })}>
      {i18n.t('reopenConversation')}
    </button>
  </div>

  {#each blocks as block (block.id)}
    {#if block.view === 'live'}
      <article class="live" data-testid="lifecycle-live" data-block={block.id}>
        <header>
          <strong>{block.title}</strong>
          <span class="num">v{block.version}{block.webgl ? ' · WebGL' : ''}</span>
          <button data-testid="lifecycle-offscreen" onclick={() => offscreen(block)}>{i18n.t('offscreen')}</button>
        </header>
        {#if restored[block.id]}<p class="restored" data-testid="artifact-restored">state restored</p>{/if}
        <iframe
          data-testid="lifecycle-iframe"
          title="Sandboxed artifact {block.id}"
          sandbox="allow-scripts"
          src={block.uri}
        ></iframe>
      </article>
    {:else}
      <article class="card" data-testid="artifact-card" data-block={block.id} data-view={block.view}>
        <header>
          <strong>{block.title}</strong>
          <span class="num">v{block.version}</span>
        </header>
        <p class="summary">{block.summary}</p>
        {#if restored[block.id]}<p class="restored" data-testid="artifact-restored">state restored</p>{/if}
        {#if block.view === 'parked'}
          <button data-testid="artifact-run" onclick={() => run(block)}>{i18n.t('runArtifact')}</button>
        {:else if block.view === 'slots_full'}
          <p class="note">{i18n.t('slotsFull')}</p>
        {:else if block.view === 'froze_view'}
          <p class="note">{i18n.t('frozeView')}</p>
          <div class="actions">
            <button data-testid="artifact-retry" onclick={() => run(block)}>{i18n.t('retry')}</button>
            <button data-testid="artifact-open-window" onclick={() => openWindow(block)}>{i18n.t('openWindow')}</button>
            <button data-testid="artifact-view-code" onclick={() => (showCode = { ...showCode, [block.id]: !showCode[block.id] })}>
              {i18n.t('viewCode')}
            </button>
          </div>
          {#if showCode[block.id]}<pre class="code" data-testid="artifact-code">{block.uri}</pre>{/if}
        {:else if block.view === 'in_window'}
          <button data-testid="artifact-close-window" onclick={() => dispatch({ type: 'CLOSE_WINDOW', id: block.id })}>
            {i18n.t('closeWindow')}
          </button>
        {:else if block.view === 'interrupted'}
          <p class="note">{i18n.t('interruptedArtifact')}</p>
          <button data-testid="artifact-ask-continue" onclick={() => dispatch({ type: 'RELOAD_CONVERSATION' })}>
            {i18n.t('askModelContinue')}
          </button>
        {:else if block.view === 'streaming'}
          <p class="note">{i18n.t('streamingArtifact')}</p>
        {/if}
      </article>
    {/if}
  {/each}
</section>

<style>
  .lifecycle { display: flex; flex-direction: column; gap: 10px; width: 100%; max-width: 1040px; }
  .bar { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
  h2 { margin: 0; font-size: 18px; font-weight: 600; }
  button { padding: 6px 10px; border: 1px solid var(--line); border-radius: 6px; color: var(--text); font-size: 12px; }
  button:hover { background: var(--raise); }
  .live, .card { padding: 12px 14px; border: 1px solid var(--line); border-radius: 10px; background: var(--surface); }
  .live { border-color: var(--flow); }
  header { display: flex; align-items: center; gap: 12px; }
  header strong { flex: 1; }
  .num { color: var(--mute); font-family: var(--mono); font-size: 11px; }
  .summary, .note { margin: 6px 0; color: var(--mute); font-size: 13px; }
  .restored { margin: 6px 0; color: var(--ok); font-size: 12px; }
  .actions { display: flex; flex-wrap: wrap; gap: 6px; }
  .card[data-view='froze_view'] { border-color: var(--attn); }
  iframe { width: 100%; height: 180px; margin-top: 8px; border: 1px solid var(--line); border-radius: 8px; background: var(--bg); }
  .code { margin: 8px 0 0; padding: 10px; overflow: auto; border-radius: 6px; background: var(--bg); font-family: var(--mono); font-size: 12px; }
</style>

<script lang="ts">
  import { onMount } from 'svelte';
  import Activity from './lib/Activity.svelte';
  import Composer from './lib/Composer.svelte';
  import Connections from './lib/Connections.svelte';
  import Diagnostics from './lib/Diagnostics.svelte';
  import { createConnectionIPC, type ModelInfo } from './lib/connections';
  import EmptyState from './lib/EmptyState.svelte';
  import InterruptedRun from './lib/InterruptedRun.svelte';
  import Icon from './lib/Icon.svelte';
  import Onboarding from './lib/Onboarding.svelte';
  import Sidebar from './lib/Sidebar.svelte';
  import { i18n } from './lib/i18n.svelte';
  import { createIPC, type IPC } from './lib/ipc';
  import {
    completeOnboarding,
    createOnboardingIPC,
    shouldShowOnboarding,
    type ExamplePrompt,
  } from './lib/onboarding';
  import { createModeIPC, modeLabel, type Mode, type ModeIPC } from './lib/modes';
  import { ChatRun } from './lib/run.svelte';
  import { theme } from './lib/theme.svelte';
  import { recordUsageEvent } from './lib/usage';
  import { usageSettings } from './lib/usage-settings.svelte';
  import Usage from './lib/Usage.svelte';
  import type { Conversation, ConversationDetail, Project, RunEvent, RunRecord, SearchResult, ShellView } from './lib/types';

  let ipc = $state<IPC | null>(null);
  let conversations = $state<Conversation[]>([]);
  let projects = $state<Project[]>([]);
  let selectedId = $state('');
  let detail = $state<ConversationDetail | null>(null);
  let draft = $state('');
  let run = $state(new ChatRun());
  let runs = $state<RunRecord[]>([]);
  let interrupted = $derived.by(() => {
    const stale = runs.find(
      (record) => record.conversationId === selectedId && record.status === 'interrupted',
    );
    // Once a retry is tied to the original run, Continue is no longer offered.
    if (!stale || runs.some((record) => record.originalRunId === stale.runId)) return null;
    return stale;
  });

  let showConnections = $state(false);
  let collapsed = $state(false);
  let view = $state<ShellView>('chat');
  let searchQuery = $state('');
  let searchResults = $state<SearchResult[]>([]);
  let searched = $state(false);
  let renaming = $state(false);
  let renameDraft = $state('');
  let moving = $state(false);
  let moveProject = $state('');
  let showOnboarding = $state(false);
  let examplePrompts = $state<ExamplePrompt[]>([]);
  let hoveredAnswer = $state('');
  let showDiagnostics = $state(false);
  let modes = $state<Mode[]>([]);
  let models = $state<ModelInfo[]>([]);
  let modeId = $state('chat');
  let modelId = $state('');
  // Injected once at mount; not reactive state (the shell never swaps it).
  let modeIPC: ModeIPC | null = null;

  // A conversation without a project has no root_uri, so no folder action.
  let hasFolder = $derived.by(() => {
    const id = detail?.conversation.projectId;
    if (!id) return false;
    const project = projects.find((p) => p.id === id);
    return !!project && project.rootUri !== '';
  });

  onMount(async () => {
    ipc = createIPC();
    modeIPC = createModeIPC(ipc);
    const onboarding = createOnboardingIPC();
    const connections = createConnectionIPC();
    const [recent, projectList, prompts, modeList, profiles] = await Promise.all([
      ipc.listRecentConversations('', 50),
      ipc.listProjects(),
      onboarding.listExamplePrompts(),
      modeIPC.listModes(),
      connections.listProfiles(),
    ]);
    examplePrompts = prompts;
    modes = modeList;
    if (profiles.length > 0) {
      models = await connections.listModels(profiles[0].id);
      modelId = models[0]?.id ?? 'gpt-test';
    }
    showOnboarding = shouldShowOnboarding();
    conversations = recent.items;
    projects = projectList;
    if (conversations.length === 0) {
      const created = await ipc.createConversation('p1', i18n.t('newTitle'), 'chat');
      conversations = [created];
    }
    const first = conversations[0];
    selectedId = first.id;
    detail = await ipc.getConversation(first.id);
    moveProject = detail.conversation.projectId;
    await refreshRuns();
  });

  async function refreshRuns() {
    if (!ipc || !selectedId) return;
    runs = await ipc.listRuns(selectedId);
  }

  // On terminal, reload canonical history from the typed binding so the
  // streamed answer is replaced by the persisted message. Run state is also
  // refreshed so a recovered run can offer Continue.
  $effect(() => {
    if (ipc && run.runId && !run.streaming) {
      const id = selectedId;
      void ipc.getConversation(id).then((loaded) => { detail = loaded; });
      void refreshRuns();
    }
  });

  function shortcut(event: KeyboardEvent) {
    if ((event.metaKey || event.ctrlKey) && event.key === '\\') {
      event.preventDefault();
      collapsed = !collapsed;
    }
  }

  async function select(id: string) {
    if (!ipc || id === selectedId) return;
    selectedId = id;
    detail = await ipc.getConversation(id);
    moveProject = detail.conversation.projectId;
    renaming = false;
    moving = false;
    await refreshRuns();
  }

  async function cancel() {
    if (!ipc || !run.runId) return;
    await ipc.cancelRun(run.runId);
  }

  // Live events feed both the chat run and the Diagnostics usage log. Restore
  // replays go through run.restore instead, so history is never double-counted.
  function applyEvent(event: RunEvent): void {
    run.apply(event);
    recordUsageEvent(event);
  }

  async function continueRun() {
    if (!ipc || !interrupted) return;
    const original = interrupted;
    run.begin();
    const handle = await ipc.resumeRun(original.runId, applyEvent);
    run.setRunId(handle.runId);
    await refreshRuns();
  }

  async function newChat() {
    if (!ipc) return;
    view = 'chat';
    const created = await ipc.createConversation('p1', i18n.t('newTitle'), 'chat');
    conversations = [created, ...conversations];
    selectedId = created.id;
    detail = await ipc.getConversation(created.id);
    moveProject = created.projectId;
    renaming = false;
    moving = false;
  }

  async function estimateSelection(kind: 'mode' | 'model', value: string) {
    if (!modeIPC) return { estimatedTokens: 0, warn: false, estimated: true };
    return modeIPC.estimateSwitch({
      conversationId: selectedId,
      modelRef: kind === 'model' ? value : modelId,
      modeId: kind === 'mode' ? value : modeId,
    });
  }

  function selectChoice(kind: 'mode' | 'model', value: string) {
    if (kind === 'mode') modeId = value;
    else modelId = value;
  }

  async function send(text: string) {
    if (!ipc || !detail || run.streaming) return;
    const userMessage = {
      id: `local-${Date.now()}`,
      conversationId: selectedId,
      role: 'user' as const,
      text,
      createdAt: new Date().toISOString(),
    };
    detail.messages = [...detail.messages, userMessage];
    run.begin();
    const handle = await ipc.startRun(
      { conversationId: selectedId, text, modelRef: modelId || 'gpt-test', harnessId: modeId },
      applyEvent,
    );
    run.setRunId(handle.runId);
  }

  async function search() {
    if (!ipc) return;
    searched = true;
    searchResults = await ipc.searchConversations(searchQuery.trim(), 20);
  }

  async function openResult(result: SearchResult) {
    view = 'chat';
    await select(result.conversationId);
  }

  function startRename() {
    if (!detail) return;
    renameDraft = detail.conversation.title;
    renaming = true;
    moving = false;
  }

  async function saveRename(event: SubmitEvent) {
    event.preventDefault();
    if (!ipc || !detail) return;
    const title = renameDraft.trim();
    if (!title) return;
    const updated = await ipc.renameConversation(detail.conversation.id, title);
    detail = { ...detail, conversation: updated };
    conversations = conversations.map((c) => (c.id === updated.id ? updated : c));
    searchResults = searchResults.map((r) => (r.conversationId === updated.id ? { ...r, title } : r));
    renaming = false;
  }

  async function saveMove() {
    if (!ipc || !detail) return;
    const updated = await ipc.moveConversation(detail.conversation.id, moveProject);
    detail = { ...detail, conversation: updated };
    conversations = conversations.map((c) => (c.id === updated.id ? updated : c));
    moving = false;
  }
</script>

<svelte:window onkeydown={shortcut} />

<div class="shell">
  <Sidebar
    bind:collapsed
    bind:view
    {conversations}
    {selectedId}
    onnewchat={() => void newChat()}
    onselect={(id) => void select(id)}
  />

  <main class="panel">
    {#if view === 'chat'}
      <header>
        <div class="title-block">
          <h1>{detail?.conversation.title ?? i18n.t('newChat')}</h1>
          <span class="mode">{modeLabel(modes.find((m) => m.id === modeId), i18n.locale)}</span>
        </div>
        <div class="actions">
          {#if hasFolder}
            <button class="text-button" data-testid="open-folder" title={i18n.t('openFolder')}>
              <Icon name="folder" size={15} />
              <span>{i18n.t('openFolder')}</span>
            </button>
          {/if}
          <button class="text-button" data-testid="rename-button" onclick={startRename}>{i18n.t('rename')}</button>
          <button class="text-button" data-testid="move-button" onclick={() => { moving = !moving; renaming = false; }}>
            {i18n.t('move')}
          </button>
          <button
            class="icon-button"
            data-testid="toggle-theme"
            aria-label={i18n.t('theme')}
            title={i18n.t('theme')}
            onclick={() => theme.toggle()}
          >
            <Icon name={theme.theme === 'dark' ? 'sun' : 'moon'} size={16} />
          </button>
          <button class="icon-button lang" data-testid="toggle-language" aria-label={i18n.t('language')} onclick={() => i18n.toggle()}>
            {i18n.locale === 'id' ? 'EN' : 'ID'}
          </button>
        </div>
      </header>

      {#if renaming}
        <form class="inline-edit" onsubmit={saveRename}>
          <input data-testid="rename-input" bind:value={renameDraft} aria-label={i18n.t('rename')} />
          <button type="submit" data-testid="rename-save">{i18n.t('save')}</button>
          <button type="button" data-testid="rename-cancel" onclick={() => (renaming = false)}>{i18n.t('cancel')}</button>
        </form>
      {/if}
      {#if moving}
        <div class="inline-edit" data-testid="move-editor">
          <select data-testid="move-select" bind:value={moveProject} aria-label={i18n.t('project')}>
            <option value="">{i18n.t('noProject')}</option>
            {#each projects as project (project.id)}
              <option value={project.id}>{project.name}</option>
            {/each}
          </select>
          <button data-testid="move-save" onclick={() => void saveMove()}>{i18n.t('save')}</button>
          <button data-testid="move-cancel" onclick={() => (moving = false)}>{i18n.t('cancel')}</button>
        </div>
      {/if}

      <div class="thread" data-testid="thread">
        {#if interrupted}
          <InterruptedRun run={interrupted} onContinue={() => void continueRun()} />
        {/if}

        {#if detail?.messages.length === 0 && !run.answer}
          <EmptyState prompts={examplePrompts} onsuggest={(text) => void send(text)} />
        {/if}

        {#each detail?.messages ?? [] as message (message.id)}
          <article
            class="message"
            class:user={message.role === 'user'}
            data-testid={message.role === 'user' ? 'user-message' : 'assistant-answer'}
            role="group"
            onmouseenter={() => (hoveredAnswer = message.id)}
            onmouseleave={() => (hoveredAnswer = '')}
          >
            <div class="role">{message.role === 'user' ? i18n.t('you') : 'Sasmita'}</div>
            <p>{message.text}</p>
            <Usage
              usage={message.role === 'assistant' ? (message.usage ?? null) : null}
              alwaysShow={usageSettings.alwaysShow}
              open={hoveredAnswer === message.id}
            />
          </article>
        {/each}

        {#if run.streaming && run.answer}
          <article class="message streaming" data-testid="streaming-answer">
            <div class="role">Sasmita</div>
            <p>{run.answer}</p>
            <Usage usage={run.usage} alwaysShow={usageSettings.alwaysShow} />
          </article>
        {/if}

        {#if run.streaming}
          <Activity status={run.status} cancelling={run.cancelling} steps={run.steps} elapsedSeconds={run.elapsedSeconds} onCancel={() => void cancel()} />
        {/if}

        {#if run.error}
          <p class="error" data-testid="run-error">{run.error}</p>
        {/if}
      </div>

      <Composer
        bind:value={draft}
        onsubmit={send}
        {modes}
        {models}
        {modeId}
        {modelId}
        estimate={estimateSelection}
        onselect={selectChoice}
      />
    {:else if view === 'search'}
      <section class="view" data-testid="search-view">
        <h1>{i18n.t('searchConversations')}</h1>
        <form class="search-form" onsubmit={(e) => { e.preventDefault(); void search(); }}>
          <input data-testid="search-input" bind:value={searchQuery} aria-label={i18n.t('searchConversations')} />
          <button type="submit" data-testid="search-submit">{i18n.t('search')}</button>
        </form>
        <div class="results" data-testid="search-results">
          {#each searchResults as result (result.conversationId + result.snippet)}
            <button class="result" data-testid="search-result" onclick={() => void openResult(result)}>
              <span class="result-title">{result.title}</span>
              <span class="result-snippet">{result.snippet}</span>
            </button>
          {/each}
          {#if searched && searchResults.length === 0}
            <p class="empty-note" data-testid="search-empty">{i18n.t('noResults')}</p>
          {/if}
        </div>
      </section>
    {:else if view === 'projects'}
      <section class="view" data-testid="projects-view">
        <h1>{i18n.t('projects')}</h1>
        {#each projects as project (project.id)}
          <div class="project-row" data-testid="project-row">
            <span>{project.name}</span>
            <span class="num">{i18n.formatNumber(conversations.filter((c) => c.projectId === project.id).length)}</span>
          </div>
        {/each}
        {#if projects.length === 0}
          <p class="empty-note">{i18n.t('noProjects')}</p>
        {/if}
      </section>
    {:else if view === 'artifacts'}
      <section class="view" data-testid="artifacts-view">
        <h1>{i18n.t('artifacts')}</h1>
        <p class="empty-note">{i18n.t('noArtifacts')}</p>
      </section>
    {:else}
      <section class="view" data-testid="settings-view">
        <h1>{i18n.t('settings')}</h1>
        <div class="setting-row">
          <span>{i18n.t('theme')}</span>
          <div class="segmented">
            <button data-testid="theme-dark" class:active={theme.theme === 'dark'} onclick={() => theme.set('dark')}>{i18n.t('dark')}</button>
            <button data-testid="theme-light" class:active={theme.theme === 'light'} onclick={() => theme.set('light')}>{i18n.t('light')}</button>
          </div>
        </div>
        <div class="setting-row">
          <span>{i18n.t('language')}</span>
          <div class="segmented">
            <button data-testid="lang-id" class:active={i18n.locale === 'id'} onclick={() => i18n.set('id')}>{i18n.t('indonesian')}</button>
            <button data-testid="lang-en" class:active={i18n.locale === 'en'} onclick={() => i18n.set('en')}>{i18n.t('english')}</button>
          </div>
        </div>
        <div class="setting-row">
          <span>{i18n.t('alwaysShowUsage')}</span>
          <button
            class="connections-button"
            data-testid="toggle-always-usage"
            aria-pressed={usageSettings.alwaysShow}
            onclick={() => usageSettings.toggle()}
          >{usageSettings.alwaysShow ? i18n.t('enabled') : i18n.t('disabled')}</button>
        </div>
        <div class="setting-row">
          <span>{i18n.t('diagnostics')}</span>
          <button
            class="connections-button"
            data-testid="open-diagnostics"
            onclick={() => (showDiagnostics = !showDiagnostics)}
          >{showDiagnostics ? i18n.t('close') : i18n.t('open')}</button>
        </div>
        {#if showDiagnostics}<Diagnostics />{/if}
        <div class="setting-row">
          <span>Provider connections</span>
          <button
            class="connections-button"
            data-testid="open-connections"
            onclick={() => (showConnections = !showConnections)}
          >{showConnections ? i18n.t('cancel') : 'Koneksi'}</button>
        </div>
        {#if showConnections}<Connections onClose={() => (showConnections = false)} />{/if}
      </section>
    {/if}
  </main>

  {#if showOnboarding}
    <Onboarding onDone={() => { completeOnboarding(); showOnboarding = false; }} />
  {/if}
</div>

<style>
  .shell { display: flex; gap: 8px; height: 100vh; padding: 8px; background: var(--ground); }
  .panel { display: flex; flex: 1; min-width: 0; flex-direction: column; border: 1px solid var(--line); border-radius: 12px; background: var(--bg); overflow: hidden; }
  header { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 14px 24px; border-bottom: 1px solid var(--line); }
  .title-block { display: flex; align-items: baseline; gap: 12px; min-width: 0; }
  h1 { margin: 0; font-size: 20px; font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .mode { color: var(--mute); font-size: 13px; white-space: nowrap; }
  .actions { display: flex; align-items: center; gap: 4px; flex: none; }
  .text-button, .icon-button { display: inline-flex; align-items: center; gap: 6px; padding: 7px 10px; border-radius: 6px; color: var(--mute); font-size: 13px; }
  .icon-button { padding: 7px; }
  .icon-button.lang { font-family: var(--mono); font-size: 11px; letter-spacing: 0.04em; }
  .text-button:hover, .icon-button:hover { background: var(--raise); color: var(--text); }
  .connections-button { padding: 8px 14px; border: 1px solid var(--line); border-radius: 6px; background: var(--surface); color: var(--text); }
  .connections-button:hover { background: var(--raise); }
  .inline-edit { display: flex; gap: 8px; padding: 10px 24px; border-bottom: 1px solid var(--line); }
  .inline-edit input, .inline-edit select, .search-form input {
    flex: 1; min-width: 0; padding: 8px 10px; border: 1px solid var(--line); border-radius: 6px;
    background: var(--surface); color: var(--text); font: inherit;
  }
  .inline-edit button, .search-form button { padding: 8px 14px; border-radius: 6px; background: var(--raise); color: var(--text); }
  .thread { flex: 1; overflow-y: auto; padding: 24px; }
  .message { position: relative; max-width: 760px; margin: 0 0 20px; padding: 14px 16px; border: 1px solid var(--line); border-radius: 10px; background: var(--surface); }
  .message.user { margin-left: auto; background: var(--raise); }
  .message.streaming { border-color: var(--flow); }
  .role { margin-bottom: 6px; color: var(--mute); font-size: 12px; }
  .message p { white-space: pre-wrap; line-height: 1.6; }
  .error { color: var(--attn); }
  .panel > :global(form.composer) { margin: 0 24px 24px; }
  .view { flex: 1; overflow-y: auto; padding: 24px; max-width: 760px; width: 100%; margin: 0 auto; }
  .search-form { display: flex; gap: 8px; margin-bottom: 16px; }
  .results { display: flex; flex-direction: column; gap: 6px; }
  .result { display: flex; flex-direction: column; gap: 4px; padding: 12px 14px; border: 1px solid var(--line); border-radius: 10px; background: var(--surface); text-align: left; }
  .result:hover { background: var(--raise); }
  .result-title { color: var(--text); font-weight: 600; }
  .result-snippet { color: var(--mute); font-size: 13px; }
  .empty-note { color: var(--mute); }
  .project-row, .setting-row { display: flex; align-items: center; justify-content: space-between; padding: 12px 0; border-bottom: 1px solid var(--line); }
  .project-row .num { color: var(--mute); font-size: 13px; }
  .segmented { display: inline-flex; border: 1px solid var(--line); border-radius: 6px; overflow: hidden; }
  .segmented button { padding: 7px 12px; color: var(--mute); }
  .segmented button.active { background: var(--raise); color: var(--text); }
</style>

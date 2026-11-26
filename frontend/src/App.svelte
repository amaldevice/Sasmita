<script lang="ts">
  import { onMount } from 'svelte';
  import Activity from './lib/Activity.svelte';
  import Composer from './lib/Composer.svelte';
  import InterruptedRun from './lib/InterruptedRun.svelte';
  import { createIPC, type IPC } from './lib/ipc';
  import { ChatRun } from './lib/run.svelte';
  import type { Conversation, ConversationDetail, Message, RunRecord } from './lib/types';

  let ipc = $state<IPC | null>(null);
  let conversations = $state<Conversation[]>([]);
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

  onMount(async () => {
    ipc = createIPC();
    const page = await ipc.listConversations('p1', '', 20);
    conversations = page.items;
    if (conversations.length === 0) {
      const created = await ipc.createConversation('p1', 'Percakapan baru', 'chat');
      conversations = [created];
    }
    selectedId = conversations[0].id;
    detail = await ipc.getConversation(selectedId);
    await refreshRuns();
  });

  async function refreshRuns() {
    if (!ipc || !selectedId) return;
    runs = await ipc.listRuns(selectedId);
  }

  // On terminal, reload the canonical history from the typed binding so the
  // streamed answer is replaced by the persisted message. Run state is also
  // refreshed so a recovered run can offer Continue.
  $effect(() => {
    if (ipc && run.runId && !run.streaming) {
      const id = selectedId;
      void ipc.getConversation(id).then((loaded) => { detail = loaded; });
      void refreshRuns();
    }
  });

  async function select(id: string) {
    if (!ipc || id === selectedId) return;
    selectedId = id;
    detail = await ipc.getConversation(id);
    await refreshRuns();
  }

  async function cancel() {
    if (!ipc || !run.runId) return;
    await ipc.cancelRun(run.runId);
  }

  async function continueRun() {
    if (!ipc || !interrupted) return;
    const original = interrupted;
    run.begin();
    const handle = await ipc.resumeRun(original.runId, (event) => run.apply(event));
    run.setRunId(handle.runId);
    await refreshRuns();
  }

  async function send(text: string) {
    if (!ipc || !detail || run.streaming) return;
    const userMessage: Message = {
      id: `local-${Date.now()}`,
      conversationId: selectedId,
      role: 'user',
      text,
      createdAt: new Date().toISOString(),
    };
    detail.messages = [...detail.messages, userMessage];
    run.begin();
    const handle = await ipc.startRun(
      { conversationId: selectedId, text, modelRef: 'gpt-test', harnessId: 'chat' },
      (event) => run.apply(event),
    );
    run.setRunId(handle.runId);
  }
</script>

<div class="shell">
  <aside class="sidebar">
    <div class="brand">Sasmita</div>
    <div class="label">PERCAKAPAN</div>
    <nav>
      {#each conversations as conversation (conversation.id)}
        <button
          class="conversation"
          class:active={conversation.id === selectedId}
          onclick={() => void select(conversation.id)}
        >
          {conversation.title}
        </button>
      {/each}
    </nav>
  </aside>

  <main class="panel">
    <header>
      <h1>{detail?.conversation.title ?? 'Sasmita'}</h1>
      <span class="mode">Mode Chat</span>
    </header>

    <div class="thread" data-testid="thread">
      {#if interrupted}
        <InterruptedRun run={interrupted} onContinue={() => void continueRun()} />
      {/if}

      {#if detail?.messages.length === 0 && !run.answer}
        <div class="empty">
          <h2>Mulai percakapan</h2>
          <p>Ajukan pertanyaan pertama Anda.</p>
        </div>
      {/if}

      {#each detail?.messages ?? [] as message (message.id)}
        <article class="message" class:user={message.role === 'user'} data-testid={message.role === 'user' ? 'user-message' : 'assistant-answer'}>
          <div class="role">{message.role === 'user' ? 'Anda' : 'Sasmita'}</div>
          <p>{message.text}</p>
        </article>
      {/each}

      {#if run.streaming && run.answer}
        <article class="message streaming" data-testid="streaming-answer">
          <div class="role">Sasmita</div>
          <p>{run.answer}</p>
        </article>
      {/if}

      {#if run.streaming}
        <Activity status={run.status} cancelling={run.cancelling} onCancel={() => void cancel()} />
      {/if}

      {#if run.usage}
        <div class="usage num" data-testid="usage">
          {run.usage.inputTokens} ↑ &nbsp;{run.usage.outputTokens} ↓ &nbsp;{run.usage.cacheReadTokens} cache
        </div>
      {/if}

      {#if run.error}
        <p class="error" data-testid="run-error">{run.error}</p>
      {/if}
    </div>

    <Composer bind:value={draft} onsubmit={send} />
  </main>
</div>

<style>
  .shell { display: flex; gap: 8px; height: 100vh; padding: 8px; background: var(--ground); }
  .sidebar { width: 272px; flex: none; padding: 16px 12px; border: 1px solid var(--line); border-radius: 12px; background: var(--side); }
  .brand { padding: 4px 8px 20px; font-size: 18px; font-weight: 700; }
  .label { padding: 0 8px 8px; color: var(--mute); font-family: var(--mono); font-size: 11px; letter-spacing: 0.08em; }
  .conversation { display: block; width: 100%; padding: 9px 10px; border-radius: 6px; color: var(--mute); text-align: left; }
  .conversation:hover { background: var(--raise); color: var(--text); }
  .conversation.active { background: var(--raise); color: var(--text); }
  .panel { display: flex; flex: 1; min-width: 0; flex-direction: column; border: 1px solid var(--line); border-radius: 12px; background: var(--bg); }
  header { display: flex; align-items: baseline; justify-content: space-between; padding: 18px 24px; border-bottom: 1px solid var(--line); }
  h1 { font-size: 20px; font-weight: 600; }
  .mode { color: var(--mute); font-size: 13px; }
  .thread { flex: 1; overflow-y: auto; padding: 24px; }
  .empty { margin: 15vh auto 0; max-width: 70ch; text-align: center; color: var(--mute); }
  .empty h2 { margin-bottom: 8px; color: var(--text); font-size: 24px; font-weight: 600; }
  .message { max-width: 760px; margin: 0 0 20px; padding: 14px 16px; border: 1px solid var(--line); border-radius: 10px; background: var(--surface); }
  .message.user { margin-left: auto; background: var(--raise); }
  .message.streaming { border-color: var(--flow); }
  .role { margin-bottom: 6px; color: var(--mute); font-size: 12px; }
  .message p { white-space: pre-wrap; line-height: 1.6; }
  .usage { margin: -8px 0 16px; color: var(--mute); font-size: 12px; }
  .error { color: var(--attn); }
  .panel > :global(form.composer) { margin: 0 24px 24px; }
</style>

<script lang="ts">
  import Icon from './Icon.svelte';
  import { i18n, type StringKey } from './i18n.svelte';
  import type { Conversation, ShellView, SidebarBadge } from './types';

  let {
    collapsed = $bindable(false),
    view = $bindable<ShellView>('chat'),
    conversations = [],
    selectedId = '',
    badges = {},
    /** Baca switches to the rail instantly; no reading-view transition. */
    instant = false,
    onnewchat,
    onselect,
  }: {
    collapsed?: boolean;
    view?: ShellView;
    conversations: Conversation[];
    selectedId: string;
    badges?: Record<string, SidebarBadge>;
    instant?: boolean;
    onnewchat: () => void;
    onselect: (id: string) => void;
  } = $props();

  const items: { view: ShellView; icon: string; key: StringKey; test: string }[] = [
    { view: 'chat', icon: 'plus', key: 'newChat', test: 'nav-new-chat' },
    { view: 'search', icon: 'search', key: 'search', test: 'nav-search' },
    { view: 'projects', icon: 'folder', key: 'projects', test: 'nav-projects' },
    { view: 'artifacts', icon: 'grid', key: 'artifacts', test: 'nav-artifacts' },
    { view: 'settings', icon: 'settings', key: 'settings', test: 'nav-settings' },
  ];

  function choose(next: ShellView) {
    if (next === 'chat') onnewchat();
    else view = next;
  }

  function open(id: string) {
    view = 'chat';
    onselect(id);
  }
</script>

<aside
  class="sidebar"
  class:collapsed
  data-testid="sidebar"
  data-state={collapsed ? 'rail' : 'expanded'}
  data-instant={instant ? 'true' : undefined}
>
  <div class="brand-row">
    {#if !collapsed}
      <div class="brand-group">
        <span class="brand-mark" aria-hidden="true"></span>
        <span class="brand">Sasmita</span>
      </div>
    {/if}
    <button
      class="icon-button"
      data-testid="collapse-button"
      aria-label={collapsed ? i18n.t('expand') : i18n.t('collapse')}
      title={collapsed ? i18n.t('expand') : i18n.t('collapse')}
      onclick={() => (collapsed = !collapsed)}
    >
      {#if collapsed}
        <span class="brand-mark rail-mark" aria-hidden="true"></span>
      {:else}
        <Icon name="chevron" size={16} />
      {/if}
    </button>
  </div>

  <nav aria-label={i18n.t('newChat')}>
    {#each items as item (item.view)}
      <button
        class="nav-item"
        class:active={view === item.view}
        data-testid={item.test}
        aria-label={i18n.t(item.key)}
        title={i18n.t(item.key)}
        onclick={() => choose(item.view)}
      >
        <Icon name={item.icon} />
        <span class="label-text">{i18n.t(item.key)}</span>
      </button>
    {/each}
  </nav>

  <div class="section-label">{i18n.t('recent')}</div>
  <div class="recent">
    {#each conversations as conversation (conversation.id)}
      <button
        class="conversation"
        class:active={conversation.id === selectedId && view === 'chat'}
        data-testid="recent-conversation"
        title={conversation.title}
        aria-current={conversation.id === selectedId ? 'true' : undefined}
        onclick={() => open(conversation.id)}
      >
        <span class="dot"></span>
        <span class="label-text">{conversation.title}</span>
        {#if badges[conversation.id]}
          <span
            class="badge"
            class:finished={badges[conversation.id] === 'finished'}
            data-testid="sidebar-badge"
            data-kind={badges[conversation.id]}
            title={badges[conversation.id] === 'waiting' ? i18n.t('waitingElsewhere') : i18n.t('finishedElsewhere')}
          ></span>
        {/if}
      </button>
    {/each}
  </div>
</aside>

<style>
  .sidebar {
    width: 272px;
    flex: none;
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 16px 12px;
    border: 1px solid var(--line);
    border-radius: 12px;
    background: var(--side);
    overflow: hidden;
    transition: width 220ms var(--ease-out), padding 220ms var(--ease-out);
  }
  .sidebar.collapsed { width: 64px; padding: 16px 10px; }
  .sidebar[data-instant='true'] { transition: none; }
  .brand-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    min-height: 28px;
    padding: 2px 8px 16px;
  }
  .collapsed .brand-row { justify-content: center; padding: 2px 0 16px; }
  .brand-group { display: flex; align-items: center; gap: 10px; min-width: 0; }
  .brand { font-size: 18px; font-weight: 700; white-space: nowrap; }
  .brand-mark {
    width: 18px;
    height: 25px;
    flex: none;
    background: var(--text);
    -webkit-mask: url('/sasmita-mark.png') center / contain no-repeat;
    mask: url('/sasmita-mark.png') center / contain no-repeat;
  }
  .rail-mark { width: 20px; height: 28px; }
  .icon-button {
    display: grid;
    place-items: center;
    width: 28px;
    height: 28px;
    flex: none;
    border-radius: 6px;
    color: var(--mute);
  }
  .icon-button:hover { background: var(--raise); color: var(--text); }
  nav, .recent { display: flex; flex-direction: column; gap: 2px; }
  .recent { overflow-y: auto; }
  .nav-item, .conversation {
    display: flex;
    align-items: center;
    gap: 12px;
    width: 100%;
    padding: 9px 10px;
    border-radius: 6px;
    color: var(--mute);
    text-align: left;
    white-space: nowrap;
  }
  .nav-item:hover, .nav-item.active,
  .conversation:hover, .conversation.active { background: var(--raise); color: var(--text); }
  .collapsed .nav-item, .collapsed .conversation { justify-content: center; padding: 9px 0; }
  .label-text { overflow: hidden; text-overflow: ellipsis; }
  .collapsed .label-text, .collapsed .section-label { display: none; }
  .section-label {
    padding: 18px 10px 6px;
    color: var(--mute);
    font-family: var(--mono);
    font-size: 11px;
    letter-spacing: 0.08em;
  }
  .dot { width: 4px; height: 4px; flex: none; border-radius: 50%; background: currentColor; }
  .badge { width: 7px; height: 7px; flex: none; border-radius: 50%; background: var(--attn); }
  .badge.finished { background: var(--ok); }
</style>

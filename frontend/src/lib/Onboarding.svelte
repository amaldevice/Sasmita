<script lang="ts">
  import { onMount } from 'svelte';
  import { createConnectionIPC, type ConnectionIPC, type ConnectionResult, type ProviderKind } from './connections';
  import { i18n } from './i18n.svelte';
  import {
    createOnboardingIPC,
    onboardingCopy,
    type LocalEndpoint,
  } from './onboarding';

  let { onDone }: { onDone: () => void } = $props();

  const onboarding = createOnboardingIPC();
  // Reuse #16's provider port: addProfile hands the key to the Keychain and
  // testConnection is the existing typed command. No new provider plumbing.
  const connections: ConnectionIPC = createConnectionIPC();

  const copy = $derived(onboardingCopy[i18n.locale]);
  let step = $state<'welcome' | 'api'>('welcome');
  let endpoints = $state<LocalEndpoint[]>([]);
  let kind = $state<ProviderKind>('openai-compatible');
  let name = $state('');
  let baseUrl = $state('');
  let apiKey = $state('');
  let result = $state<ConnectionResult | null>(null);
  let error = $state('');
  let busy = $state(false);

  onMount(() => {
    void onboarding.detectLocalEndpoints().then((found) => { endpoints = found; });
  });

  function localHost(raw: string): boolean {
    try {
      return ['127.0.0.1', 'localhost', '::1'].includes(new URL(raw).hostname);
    } catch {
      return false;
    }
  }

  function useLocal(endpoint: LocalEndpoint) {
    kind = 'openai-compatible';
    name = endpoint.name;
    baseUrl = endpoint.apiBaseUrl;
    apiKey = '';
    result = null;
    error = '';
    step = 'api';
  }

  async function saveAndTest() {
    if (busy) return;
    error = '';
    result = null;
    if (!name.trim() || !baseUrl.trim()) {
      error = copy.fieldsRequired;
      return;
    }
    if (!apiKey.trim() && !localHost(baseUrl.trim())) {
      error = copy.keyRequired;
      return;
    }
    busy = true;
    try {
      const profile = await connections.addProfile({
        kind,
        displayName: name.trim(),
        baseUrl: baseUrl.trim(),
        apiKey,
      });
      result = await connections.testConnection(profile.id);
    } catch (cause) {
      result = { ok: false, message: cause instanceof Error ? cause.message : String(cause) };
    } finally {
      busy = false;
    }
  }
</script>

<div class="backdrop" data-testid="onboarding" role="dialog" aria-modal="true" aria-label={copy.dialogLabel}>
  <section class="card">
    {#if step === 'welcome'}
      <h1 data-testid="welcome-title">{copy.welcomeTitle}</h1>
      <p class="lead" data-testid="welcome-body">{copy.welcomeBody}</p>

      {#if endpoints.length > 0}
        <h2>{copy.detected}</h2>
        <div class="endpoints">
          {#each endpoints as endpoint (endpoint.kind)}
            <button type="button" class="endpoint" data-testid="detected-endpoint" onclick={() => useLocal(endpoint)}>
              <span class="endpoint-name">{endpoint.name}</span>
              <span class="num endpoint-url">{endpoint.baseUrl}</span>
              <span class="endpoint-note">{copy.notTested}</span>
            </button>
          {/each}
        </div>
      {:else}
        <p class="note" data-testid="no-local">{copy.noLocal}</p>
      {/if}

      <div class="actions">
        <button type="button" class="primary" data-testid="use-api" onclick={() => (step = 'api')}>{copy.useApi}</button>
        <button type="button" class="quiet" data-testid="onboarding-language" onclick={() => i18n.toggle()}>
          {i18n.locale === 'id' ? 'EN' : 'ID'}
        </button>
      </div>
    {:else}
      <h1 data-testid="api-title">{copy.apiTitle}</h1>
      <p class="lead">{copy.apiBody}</p>

      <form onsubmit={(event) => { event.preventDefault(); void saveAndTest(); }}>
        <label for="onboarding-kind">{copy.kindLabel}</label>
        <select id="onboarding-kind" data-testid="onboarding-kind" bind:value={kind}>
          <option value="openai-compatible">OpenAI-compatible</option>
          <option value="anthropic-compatible">Anthropic-compatible</option>
        </select>

        <label for="onboarding-name">{copy.nameLabel}</label>
        <input id="onboarding-name" data-testid="onboarding-name" bind:value={name} />

        <label for="onboarding-base-url">{copy.baseUrlLabel}</label>
        <input id="onboarding-base-url" data-testid="onboarding-base-url" bind:value={baseUrl} />

        <label for="onboarding-api-key">{copy.keyLabel}</label>
        <input id="onboarding-api-key" data-testid="onboarding-api-key" type="password" bind:value={apiKey} autocomplete="off" />
        <p class="note">{copy.localKeyHint}</p>

        <div class="actions">
          <button type="submit" class="primary" data-testid="onboarding-save-test" disabled={busy}>
            {busy ? copy.working : copy.saveAndTest}
          </button>
          <button type="button" class="quiet" data-testid="onboarding-back" onclick={() => { step = 'welcome'; result = null; error = ''; }}>
            {copy.back}
          </button>
        </div>
      </form>

      {#if error}<p class="error" data-testid="onboarding-error">{error}</p>{/if}
      {#if result}
        <p class:ok={result.ok} class:bad={!result.ok} data-testid="onboarding-result">{result.message}</p>
        {#if result.ok}
          <div class="actions">
            <button type="button" class="primary" data-testid="onboarding-continue" onclick={onDone}>{copy.continueLabel}</button>
          </div>
        {/if}
      {/if}
    {/if}
  </section>
</div>

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    z-index: 20;
    display: grid;
    place-items: center;
    padding: 24px;
    background: var(--ground);
  }
  .card {
    width: min(560px, 100%);
    max-height: calc(100vh - 48px);
    overflow-y: auto;
    padding: 32px;
    border: 1px solid var(--line);
    border-radius: 12px;
    background: var(--bg);
  }
  h1 { margin: 0; font-size: 28px; line-height: 1.3; font-weight: 600; }
  h2 {
    margin: 24px 0 8px;
    color: var(--mute);
    font-family: var(--mono);
    font-size: 12px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }
  .lead { margin: 12px 0 0; max-width: 62ch; color: var(--mute); }
  .note { margin: 10px 0 0; color: var(--mute); font-size: 13px; }
  .endpoints { display: grid; gap: 8px; }
  .endpoint {
    display: grid;
    gap: 2px;
    padding: 14px 16px;
    border: 1px solid var(--line);
    border-radius: 10px;
    background: var(--surface);
    text-align: left;
  }
  .endpoint:hover { border-color: var(--flow); background: var(--raise); }
  .endpoint-name { font-weight: 600; }
  .endpoint-url { color: var(--mute); font-size: 12px; }
  .endpoint-note { color: var(--mute); font-size: 12px; }
  .actions { display: flex; align-items: center; gap: 8px; margin-top: 24px; }
  button.primary { padding: 9px 16px; border-radius: 6px; background: var(--text); color: var(--bg); }
  button.primary:hover { opacity: 0.88; }
  button.primary:disabled { opacity: 0.5; cursor: default; }
  button.quiet { padding: 9px 12px; border: 1px solid var(--line); border-radius: 6px; background: var(--surface); color: var(--mute); font-family: var(--mono); font-size: 12px; }
  button.quiet:hover { background: var(--raise); color: var(--text); }
  form { display: grid; gap: 4px; margin-top: 20px; }
  label { margin-top: 10px; color: var(--mute); font-size: 12px; }
  input, select {
    width: 100%;
    padding: 9px 11px;
    border: 1px solid var(--line);
    border-radius: 6px;
    background: var(--surface);
    color: var(--text);
    font: inherit;
  }
  .error, .bad { color: var(--attn); }
  .ok { color: var(--ok); }
</style>

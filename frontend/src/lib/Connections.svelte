<script lang="ts">
  import { onMount } from 'svelte';
  import {
    createConnectionIPC,
    type ConnectionIPC,
    type ConnectionResult,
    type ModelInfo,
    type ProviderKind,
    type ProviderProfile,
  } from './connections';

  let { onClose }: { onClose: () => void } = $props();

  const ipc: ConnectionIPC = createConnectionIPC();
  let profiles = $state<ProviderProfile[]>([]);
  let selectedId = $state('');
  let models = $state<ModelInfo[]>([]);
  let result = $state<ConnectionResult | null>(null);
  let error = $state('');

  let name = $state('');
  let baseUrl = $state('');
  let kind = $state<ProviderKind>('anthropic-compatible');
  let apiKey = $state('');

  onMount(async () => {
    profiles = await ipc.listProfiles();
    selectedId = profiles[0]?.id ?? '';
  });

  function select(id: string) {
    selectedId = id;
    models = [];
    result = null;
  }

  async function loadModels() {
    if (!selectedId) return;
    models = await ipc.listModels(selectedId);
  }

  async function testConnection() {
    if (!selectedId) return;
    result = await ipc.testConnection(selectedId);
  }

  async function save() {
    error = '';
    if (!name.trim() || !baseUrl.trim() || !apiKey) {
      error = 'Isi nama, base URL, dan kunci API.';
      return;
    }
    try {
      const created = await ipc.addProfile({
        kind,
        displayName: name.trim(),
        baseUrl: baseUrl.trim(),
        apiKey,
      });
      profiles = [...profiles, created];
      name = '';
      baseUrl = '';
      apiKey = '';
      select(created.id);
    } catch (cause) {
      error = cause instanceof Error ? cause.message : String(cause);
    }
  }

  async function remove(id: string) {
    await ipc.deleteProfile(id);
    profiles = profiles.filter((profile) => profile.id !== id);
    if (selectedId === id) {
      selectedId = profiles[0]?.id ?? '';
      models = [];
      result = null;
    }
  }
</script>

<section class="settings" data-testid="connections-panel" aria-label="Koneksi provider">
  <header>
    <h2>Koneksi</h2>
    <button type="button" onclick={onClose} aria-label="Tutup koneksi">Tutup</button>
  </header>

  <div class="columns">
    <div>
      <h3>Profil</h3>
      {#each profiles as profile (profile.id)}
        <div class="profile" data-testid="profile-row">
          <button
            type="button"
            class="pick"
            class:active={profile.id === selectedId}
            data-testid={`select-profile-${profile.id}`}
            onclick={() => select(profile.id)}
          >
            {profile.displayName}
            <small>{profile.kind}</small>
          </button>
          <button type="button" class="remove" onclick={() => void remove(profile.id)} aria-label={`Hapus ${profile.displayName}`}>
            Hapus
          </button>
        </div>
      {/each}

      <h3>Tambah profil</h3>
      <label for="profile-name">Nama</label>
      <input id="profile-name" data-testid="profile-name" bind:value={name} />
      <label for="profile-kind">Jenis</label>
      <select id="profile-kind" bind:value={kind}>
        <option value="anthropic-compatible">Anthropic-compatible</option>
        <option value="openai-compatible">OpenAI-compatible</option>
      </select>
      <label for="profile-base-url">Base URL</label>
      <input id="profile-base-url" data-testid="profile-base-url" bind:value={baseUrl} />
      <label for="profile-api-key">Kunci API</label>
      <input id="profile-api-key" data-testid="profile-api-key" type="password" bind:value={apiKey} />
      <button type="button" data-testid="save-profile" onclick={() => void save()}>Simpan profil</button>
      {#if error}<p class="form-error">{error}</p>{/if}
    </div>

    <div>
      <h3>Model</h3>
      <div class="actions">
        <button type="button" data-testid="test-connection" onclick={() => void testConnection()}>Uji koneksi</button>
        <button type="button" data-testid="load-models" onclick={() => void loadModels()}>Muat model</button>
      </div>
      {#if result}
        <p class:result-ok={result.ok} class:result-bad={!result.ok} data-testid="connection-result">{result.message}</p>
      {/if}
      {#each models as model (model.id)}
        <div class="model" data-testid="model-row">
          <strong>{model.id}</strong>
          <span class="num">tools: {model.capabilities.tools}</span>
          <span class="num">vision: {model.capabilities.vision}</span>
          <span class="num">context: {model.capabilities.contextWindow ?? 'unknown'}</span>
        </div>
      {/each}
    </div>
  </div>
</section>

<style>
  .settings {
    position: absolute;
    inset: 8px;
    z-index: 10;
    overflow: auto;
    padding: 20px 24px;
    border: 1px solid var(--line);
    border-radius: 12px;
    background: var(--bg);
  }
  header { display: flex; align-items: baseline; justify-content: space-between; }
  h2 { font-size: 20px; font-weight: 600; }
  h3 { margin: 18px 0 8px; color: var(--mute); font-size: 12px; letter-spacing: 0.08em; text-transform: uppercase; }
  .columns { display: grid; grid-template-columns: minmax(260px, 1fr) minmax(260px, 1fr); gap: 24px; }
  label { display: block; margin-top: 10px; color: var(--mute); font-size: 12px; }
  input, select {
    width: 100%;
    margin-top: 4px;
    padding: 8px 10px;
    border: 1px solid var(--line);
    border-radius: 6px;
    background: var(--surface);
    color: var(--text);
  }
  button { padding: 7px 12px; border: 1px solid var(--line); border-radius: 6px; background: var(--surface); color: var(--text); }
  button:hover { background: var(--raise); }
  .profile { display: flex; gap: 6px; align-items: stretch; margin-bottom: 6px; }
  .pick { flex: 1; text-align: left; }
  .pick.active { border-color: var(--flow); background: var(--raise); }
  .pick small { display: block; color: var(--mute); font-size: 11px; }
  .remove { color: var(--mute); }
  .actions { display: flex; gap: 8px; }
  .model { margin-top: 10px; padding: 10px 12px; border: 1px solid var(--line); border-radius: 10px; background: var(--surface); }
  .model .num { display: inline-block; margin-right: 12px; color: var(--mute); font-size: 12px; }
  .result-ok { color: var(--ok); }
  .result-bad { color: var(--attn); }
  .form-error { margin-top: 8px; color: var(--attn); }
</style>

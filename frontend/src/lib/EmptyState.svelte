<script lang="ts">
  import { i18n } from './i18n.svelte';
  import { onboardingCopy, type ExamplePrompt } from './onboarding';

  let {
    prompts,
    onsuggest,
  }: { prompts: ExamplePrompt[]; onsuggest: (text: string) => void } = $props();

  const copy = $derived(onboardingCopy[i18n.locale]);
</script>

<div class="empty" data-testid="empty-state">
  <h2 data-testid="empty-title">{copy.emptyTitle}</h2>
  <p data-testid="empty-body">{copy.emptyBody}</p>

  <div class="cards" data-testid="suggestions">
    {#each prompts as prompt (prompt.id)}
      <button type="button" class="card" data-testid="suggestion-card" onclick={() => onsuggest(prompt.prompt)}>
        <span class="card-title">{i18n.locale === 'id' ? prompt.titleId : prompt.titleEn}</span>
        <span class="card-detail">{i18n.locale === 'id' ? prompt.detailId : prompt.detailEn}</span>
      </button>
    {/each}
  </div>
</div>

<style>
  .empty { max-width: 760px; margin: 10vh auto 0; }
  h2 { margin: 0; font-size: 28px; font-weight: 600; line-height: 1.3; }
  p { margin: 8px 0 0; color: var(--mute); }
  .cards { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; margin-top: 28px; }
  .card {
    display: grid;
    align-content: start;
    gap: 6px;
    min-height: 96px;
    padding: 16px;
    border: 1px solid var(--line);
    border-radius: 10px;
    background: var(--surface);
    text-align: left;
  }
  .card:hover { border-color: var(--flow); background: var(--raise); }
  .card-title { font-weight: 600; }
  .card-detail { color: var(--mute); font-size: 13px; }
</style>

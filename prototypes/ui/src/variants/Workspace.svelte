<script lang="ts">
  // Layout A as the base, with: artifact opened beside the chat (old B) and full screen,
  // a reading view (old C), and three formats for visual + text.
  import { fly, fade } from 'svelte/transition';
  import Chrome, { icons } from '../lib/Chrome.svelte';
  import NNViz from '../lib/NNViz.svelte';
  import Activity from '../lib/Activity.svelte';
  import StreamText from '../lib/StreamText.svelte';
  import Approval from '../lib/Approval.svelte';
  import type { Strings } from '../lib/i18n';
  import type { AnswerRun, Token } from '../lib/run.svelte';

  type Fmt = 'stack' | 'stack2' | 'side';
  let { s, run, tokens }: { s: Strings; run: AnswerRun; tokens: Token[] } = $props();

  const q = new URLSearchParams(location.search);
  let view = $state<'chat' | 'read'>(q.get('view') === 'read' ? 'read' : 'chat');
  let fmt = $state<Fmt>((q.get('fmt') as Fmt) ?? 'stack');
  let panel = $state<'off' | 'side' | 'full'>('off');
  let fmtOpen = $state(false);

  let inline = $derived(panel === 'off');
  let textCols = $derived(fmt === 'stack2' || (view === 'read' && fmt === 'stack2'));
</script>

{#snippet ic(name: string)}<svg class="ic" viewBox="0 0 24 24" aria-hidden="true"><path d={icons[name]} /></svg>{/snippet}

{#snippet artifactBar()}
  <div class="abar">
    <span class="aname">{s.live}</span>
    <div class="acts">
      <div class="menu">
        <button onclick={() => (fmtOpen = !fmtOpen)} aria-expanded={fmtOpen}>{s.format}: {s.fmt[fmt]}</button>
        {#if fmtOpen}
          <div class="pop" transition:fly={{ y: -4, duration: 160 }} role="menu">
            {#each Object.entries(s.fmt) as [k, label] (k)}
              <button role="menuitemradio" aria-checked={fmt === k} class={{ on: fmt === k }} onclick={() => { fmt = k as Fmt; fmtOpen = false; }}>
                <span class={['glyph', k]} aria-hidden="true"><i></i><i></i><i></i></span>{label}
              </button>
            {/each}
          </div>
        {/if}
      </div>
      <button onclick={() => (panel = 'side')}>{@render ic('expand')}{s.openSide}</button>
    </div>
  </div>
{/snippet}

<div class={['app', view, panel !== 'off' && 'withPanel', panel === 'full' && 'full']}>
  {#if view === 'read'}<Chrome part="rail" {s} />{:else}<Chrome part="sidebar" {s} />{/if}

  <main>
    <header>
      <b>{s.title}</b>
      <span class="sub">{s.mode}</span>
      <div class="views" role="tablist">
        <button role="tab" aria-selected={view === 'chat'} class={{ on: view === 'chat' }} onclick={() => (view = 'chat')}>{s.viewChat}</button>
        <button role="tab" aria-selected={view === 'read'} class={{ on: view === 'read' }} onclick={() => (view = 'read')}>{s.viewRead}</button>
      </div>
    </header>
    <div class="scroll">
      <div class="col">
        <div class="user" in:fly={{ y: 10, duration: 300 }}>{s.ask}</div>
        <Activity {s} {run} />
        {#if run.stage >= 3}<h2 in:fly={{ y: 8, duration: 420 }}>{s.heading}</h2>{/if}

        <div class={['answer', inline && fmt]}>
          <div class="visual">
            {#if run.stage === 2}<div out:fade={{ duration: 120 }}><Chrome part="skeleton" {s} /></div>{/if}
            {#if run.stage >= 3}
              {#if inline}
                <div in:fade={{ duration: 200 }}>{@render artifactBar()}<NNViz {s} stack={fmt === 'side'} /></div>
              {:else}
                <button class="ref" in:fade onclick={() => (panel = 'off')}>
                  <span class="pulse" aria-hidden="true"></span>{s.openedSide}
                </button>
              {/if}
            {/if}
          </div>
          {#if run.stage >= 4}<div class="words"><StreamText {tokens} shown={run.words} columns={inline && textCols} /></div>{/if}
        </div>

        {#if run.stage >= 5}<p class="usage" in:fade>{s.usage}</p>{/if}
        <Approval {s} {run} />
      </div>
    </div>
    <div class="dock"><Chrome part="composer" {s} /></div>
  </main>

  {#if panel !== 'off'}
    <section class="panel" transition:fly={{ x: 40, duration: 320 }}>
      <header>
        <b>{s.heading}</b>
        <div class="acts">
          <button onclick={() => (panel = panel === 'full' ? 'side' : 'full')}>{@render ic('expand')}{panel === 'full' ? s.exitFull : s.full}</button>
          <button onclick={() => (panel = 'off')} aria-label={s.closeSide}><svg class="ic" viewBox="0 0 24 24" aria-hidden="true"><path d="M6 6l12 12M18 6L6 18" /></svg></button>
        </div>
      </header>
      <div class="stage"><NNViz {s} big autoplay={false} /></div>
    </section>
  {/if}
</div>

<svelte:window onkeydown={(e) => { if (e.key === 'Escape') { if (panel === 'full') panel = 'side'; else fmtOpen = false; } }} />

<style>
  .ic { width: 16px; height: 16px; stroke: currentColor; fill: none; stroke-width: 1.8; stroke-linecap: round; }
  .app { display: grid; grid-template-columns: 272px 1fr; height: 100vh; transition: grid-template-columns 320ms var(--ease-out); }
  .app.read { grid-template-columns: 64px 1fr; }
  .app.withPanel { grid-template-columns: 272px minmax(380px, 1fr) 1.25fr; }
  .app.read.withPanel { grid-template-columns: 64px minmax(380px, 1fr) 1.25fr; }
  .app.full { grid-template-columns: 0 0 1fr; }
  .app.full > :global(:not(.panel)) { visibility: hidden; }

  main { display: flex; flex-direction: column; min-height: 0; min-width: 0; position: relative; }
  header { height: 56px; flex: none; display: flex; align-items: center; gap: 10px; padding: 0 24px; border-bottom: 1px solid var(--line); }
  header b { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .sub { color: var(--mute); font-size: 14px; }
  .views { margin-left: auto; display: flex; gap: 2px; padding: 2px; border-radius: 8px; background: var(--raise); }
  .views button { padding: 3px 12px; border-radius: 6px; font-size: 14px; color: var(--mute); transition: background 200ms var(--ease-out), color 200ms var(--ease-out); }
  .views button.on { background: var(--bg); color: var(--text); }

  .scroll { flex: 1; overflow: auto; padding: 26px 24px 180px; }
  .col { max-width: 760px; margin: 0 auto; transition: max-width 320ms var(--ease-out); }
  .read .col, .col:has(.answer.side) { max-width: 1040px; }
  .withPanel .col { max-width: 640px; }
  .user { margin-left: auto; width: max-content; max-width: 70%; background: var(--user); padding: 11px 16px; border-radius: 14px; }
  .read .user { margin-left: 0; background: none; padding: 0; color: var(--mute); max-width: none; }
  h2 { font: 700 30px/1.2 var(--display); letter-spacing: -0.015em; margin-bottom: 18px; max-width: 24ch; text-wrap: balance; }
  .read h2 { font-size: clamp(36px, 3.6vw, 50px); line-height: 1.08; }

  .answer.side { display: grid; grid-template-columns: 1.15fr 1fr; gap: 36px; align-items: start; }
  .answer.side .visual { position: sticky; top: 0; }
  .words { margin-top: 18px; }
  .answer.side .words { margin-top: 0; }

  .abar { white-space: nowrap; display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 10px; font-size: 13.5px; color: var(--mute); }
  .aname::before { content: ''; display: inline-block; width: 7px; height: 7px; border-radius: 50%; background: var(--ok); margin-right: 8px; vertical-align: 1px; animation: breathe 1.6s ease-in-out infinite; }
  @keyframes breathe { 50% { opacity: 0.3; } }
  .acts { display: flex; gap: 4px; align-items: center; }
  .acts button { display: inline-flex; gap: 6px; align-items: center; padding: 4px 8px; border-radius: 6px; color: var(--mute); font-size: 13.5px; }
  .acts button:hover { background: var(--raise); color: var(--text); }
  .menu { position: relative; }
  .pop { position: absolute; right: 0; top: calc(100% + 4px); z-index: 5; min-width: 210px; padding: 4px; border-radius: var(--r-md); background: var(--surface); border: 1px solid var(--line); box-shadow: 0 10px 28px -8px rgba(0, 0, 0, 0.5); }
  .pop button { width: 100%; gap: 10px; padding: 7px 8px; color: var(--text); }
  .pop button.on { background: var(--raise); }
  .glyph { display: grid; gap: 2px; width: 22px; height: 16px; flex: none; }
  .glyph i { background: var(--mute); border-radius: 1px; }
  .glyph.stack { grid-template-rows: 2fr 1fr 1fr; }
  .glyph.stack2 { grid-template: 2fr 1fr / 1fr 1fr; }
  .glyph.stack2 i:first-child { grid-column: 1 / 3; }
  .glyph.stack2 i:last-child { grid-row: 2; }
  .glyph.side { grid-template: 1fr 1fr / 1.2fr 1fr; }
  .glyph.side i:first-child { grid-row: 1 / 3; }
  .on .glyph i:first-child { background: var(--accent); }

  .ref { display: flex; gap: 10px; align-items: center; width: 100%; padding: 12px 14px; border: 1px dashed var(--line); border-radius: var(--r-md); color: var(--mute); }
  .ref:hover { color: var(--text); border-style: solid; }
  .pulse { width: 8px; height: 8px; border-radius: 50%; background: var(--ok); animation: breathe 1.6s ease-in-out infinite; }
  .usage { font-size: 13px; color: var(--mute); margin-top: 4px; }
  .dock { position: absolute; left: 0; right: 0; bottom: 20px; padding: 0 24px; }
  .dock > :global(*) { max-width: 800px; margin: 0 auto; }

  .panel { display: flex; flex-direction: column; min-width: 0; border-left: 1px solid var(--line); background: color-mix(in srgb, var(--side) 55%, var(--bg)); }
  .stage { flex: 1; overflow: auto; padding: 28px 40px; }
  .full .stage { padding: 40px max(40px, calc((100% - 1100px) / 2)); }
</style>

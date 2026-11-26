<script lang="ts">
  // PROTOTYPE: three layouts (?variant=A|B|C) x three visual worlds (?world=papan|koran|canon),
  // plus ?theme=dark|light and ?lang=id|en. Throwaway; lives on branch prototype/ui-design.
  import { onMount } from 'svelte';
  import VariantA from './variants/VariantA.svelte';
  import VariantB from './variants/VariantB.svelte';
  import VariantC from './variants/VariantC.svelte';
  import Switcher from './lib/Switcher.svelte';
  import { t, type Lang } from './lib/i18n';
  import { AnswerRun, tokenize } from './lib/run.svelte';

  const q = new URLSearchParams(location.search);
  let variant = $state(q.get('variant') ?? 'A');
  let world = $state(q.get('world') ?? 'papan');
  let theme = $state(q.get('theme') ?? 'dark');
  let lang = $state<Lang>((q.get('lang') as Lang) ?? 'id');

  let s = $derived(t(lang));
  let tokens = $derived(tokenize(s.para));
  let run = $state(new AnswerRun(0));
  let runKey = $state(0);

  function restart() {
    run.destroy();
    run = new AnswerRun(tokens.length);
    runKey += 1;
    run.start();
  }

  function set(k: 'variant' | 'world' | 'theme' | 'lang', v: string) {
    if (k === 'variant') variant = v; else if (k === 'world') world = v; else if (k === 'theme') theme = v; else lang = v as Lang;
    const p = new URLSearchParams({ variant, world, theme, lang });
    history.replaceState(null, '', `?${p}`);
    if (k === 'variant' || k === 'lang') restart();
  }

  $effect(() => {
    document.documentElement.dataset.world = world;
    document.documentElement.dataset.theme = theme;
    document.documentElement.lang = lang;
  });

  onMount(() => {
    restart();
    return () => run.destroy();
  });

  const Variants = { A: VariantA, B: VariantB, C: VariantC } as const;
  let View = $derived(Variants[variant as keyof typeof Variants] ?? VariantA);
</script>

{#key runKey}
  <View {s} {run} {tokens} {world} />
{/key}
<Switcher {variant} {world} {theme} {lang} {set} replay={restart} />

<script lang="ts">
  import { t } from '$lib/i18n';
  import LocalRarityCard from './LocalRarityCard.svelte';

  let scenario = $state('available');
  const scenarios = ['available', 'rare', 'missing', 'historical', 'unavailable'] as const;
  const sampleDate = '2026-05-01';
</script>

<svelte:head><title>{t('species.localRarity.preview.title')}</title></svelte:head>

<main class="max-w-3xl mx-auto p-6 space-y-6 text-[var(--color-base-content)]">
  <h1 class="text-2xl font-bold">{t('species.localRarity.preview.title')}</h1>
  <p class="border-l-4 border-[var(--color-primary)] bg-[var(--color-base-200)] p-4 rounded">
    {t('species.localRarity.preview.notice')}
  </p>
  <div class="space-y-2">
    <label for="rarity-scenario" class="block font-semibold"
      >{t('species.localRarity.preview.scenario')}</label
    >
    <select
      id="rarity-scenario"
      bind:value={scenario}
      class="w-full rounded border border-[var(--color-base-300)] bg-[var(--color-base-100)] p-3"
    >
      {#each scenarios as option (option)}
        <option value={option}>{t(`species.localRarity.preview.${option}`)}</option>
      {/each}
    </select>
  </div>
  {#key scenario}
    <LocalRarityCard scientificName={scenario} date={sampleDate} />
  {/key}
</main>

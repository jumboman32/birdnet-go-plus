<script lang="ts">
  import { t } from '$lib/i18n';
  import { formatNumber, formatDateTime } from '$lib/utils/formatters';
  import { buildAppUrl } from '$lib/utils/urlHelpers';
  import { localizeSpeciesName } from '$lib/utils/speciesDisplay';
  import { generateId } from '$lib/utils/uuid';
  import { getLogger } from '$lib/utils/logger';
  import BarChart from './charts/d3/BarChart.svelte';
  import {
    CONFIDENCE_BINS,
    REVIEW_PAGE_SIZE,
    confidenceSearch,
    histogramBars,
    isConfidenceReview,
    type ConfidenceSeries,
    type ConfidenceReview,
  } from '../utils/confidenceAnalytics';
  import type { AnalyticsParams } from '../registry/types';

  interface Props {
    series: ConfidenceSeries[];
    params: AnalyticsParams;
  }
  let { series, params }: Props = $props();
  const id = generateId('confidence');
  const logger = getLogger('confidence-review');
  let normalized = $state(false);
  let selectedModels = $state<number[]>([]);
  const models = $derived([...new Map(series.map(row => [row.modelId, row.model])).entries()]);
  const visible = $derived(
    series.filter(row => selectedModels.length === 0 || selectedModels.includes(row.modelId))
  );
  let review = $state<{ row: ConfidenceSeries; bin: number; offset: number } | null>(null);
  let reviewRows = $state<ConfidenceReview[]>([]);
  let reviewLoading = $state(false);
  let reviewError = $state(false);
  let reviewController: AbortController | null = null;

  function openBin(row: ConfidenceSeries, label: string) {
    const bin = histogramBars(row, false).findIndex(bar => bar.label === label);
    if (bin >= 0) review = { row, bin, offset: 0 };
  }

  $effect(() => {
    // A new response invalidates any review panel from the previous range/species selection.
    void series;
    review = null;
    selectedModels = [];
  });

  $effect(() => {
    const current = review;
    const search = confidenceSearch(params);
    reviewController?.abort();
    const controller = new AbortController();
    reviewController = controller;
    reviewRows = [];
    reviewError = false;
    reviewLoading = current !== null;
    if (!current) return () => controller.abort();
    search.set('format', 'review');
    search.delete('species');
    if (current.row.species) search.set('species', current.row.species);
    search.set('model', String(current.row.modelId));
    search.set('bin', String(current.bin));
    search.set('offset', String(current.offset));
    void fetch(buildAppUrl(`/api/v2/analytics/confidence/distribution?${search}`), {
      signal: controller.signal,
    })
      .then(async response => {
        if (!response.ok) throw new Error('Confidence review failed');
        const rows: unknown = await response.json();
        if (!Array.isArray(rows) || !rows.every(isConfidenceReview))
          throw new Error('Invalid confidence review response');
        if (!controller.signal.aborted) reviewRows = rows;
      })
      .catch((error: unknown) => {
        if (!controller.signal.aborted) {
          logger.error('Failed to load confidence review', error);
          reviewError = true;
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) reviewLoading = false;
      });
    return () => controller.abort();
  });

  const percentage = (count: number, total: number) =>
    `${(total > 0 ? (count / total) * 100 : 0).toFixed(1)}%`;
</script>

<div class="space-y-6">
  <div class="rounded-lg bg-[var(--color-base-200)]/60 p-4 text-sm space-y-2">
    <p>{t('analytics.confidenceAnalysis.caution')}</p>
    <p class="opacity-75">{t('analytics.confidenceAnalysis.threshold')}</p>
    <p class="opacity-75">{t('analytics.confidenceAnalysis.population')}</p>
  </div>
  <div class="flex flex-wrap items-center gap-4">
    <label for={`${id}-normalized`} class="flex items-center gap-2 text-sm">
      <input id={`${id}-normalized`} type="checkbox" bind:checked={normalized} />
      {t('analytics.confidenceAnalysis.normalized')}
    </label>
    <fieldset class="flex flex-wrap items-center gap-3">
      <legend class="text-sm font-medium">{t('analytics.confidenceAnalysis.models')}</legend>
      {#each models as [modelId, model] (modelId)}
        <label class="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={selectedModels.includes(modelId)}
            onchange={event => {
              selectedModels = event.currentTarget.checked
                ? [...selectedModels, modelId]
                : selectedModels.filter(value => value !== modelId);
              review = null;
            }}
          />
          {model || t('analytics.confidenceAnalysis.unknownModel')} · #{modelId}
        </label>
      {/each}
      <p class="text-xs opacity-70">{t('analytics.confidenceAnalysis.allModels')}</p>
    </fieldset>
  </div>
  {#each visible as row (`${row.modelId}-${row.species}`)}
    <section class="rounded-xl border border-[var(--color-base-200)] p-4 space-y-4">
      <div class="flex flex-wrap justify-between gap-2">
        <h4 class="font-semibold">
          {row.model || t('analytics.confidenceAnalysis.unknownModel')} · #{row.modelId}{row.species
            ? ` · ${localizeSpeciesName(row.species, row.commonName || row.species)}`
            : ''}
        </h4>
        <span class="text-sm opacity-70"
          >{t('analytics.confidenceAnalysis.sample', { count: formatNumber(row.count) })}</span
        >
      </div>
      <dl class="grid grid-cols-3 lg:grid-cols-5 gap-4 text-sm">
        <div>
          <dt class="opacity-70">{t('analytics.stats.avgConfidence')}</dt>
          <dd class="text-lg font-semibold">{(row.average * 100).toFixed(1)}%</dd>
        </div>
        <div>
          <dt class="opacity-70">{t('analytics.confidenceAnalysis.median')}</dt>
          <dd class="text-lg font-semibold">{(row.median * 100).toFixed(1)}%</dd>
        </div>
        {#each [{ label: '20–30%', count: row.low }, { label: '30–80%', count: row.middle }, { label: '80–100%', count: row.high }] as band (band.label)}
          <div>
            <dt class="opacity-70">{band.label}</dt>
            <dd class="text-lg font-semibold">{percentage(band.count, row.count)}</dd>
            <dd class="text-xs opacity-70">
              {formatNumber(band.count)} / {formatNumber(row.count)}
            </dd>
          </div>
        {/each}
      </dl>
      <p class="text-xs opacity-70">{t('analytics.confidenceAnalysis.binsHelp')}</p>
      <div class="h-80">
        <BarChart
          data={histogramBars(row, normalized)}
          height={320}
          valueAxisLabel={normalized
            ? t('analytics.confidenceAnalysis.percent')
            : t('analytics.charts.numberOfDetections')}
          categoryAxisLabel={t('analytics.advanced.charts.confidence.axisLabel')}
          formatValue={normalized ? value => `${value.toFixed(1)}%` : formatNumber}
          ariaLabel={t('analytics.advanced.charts.confidence.ariaLabel')}
          onBarClick={datum => openBin(row, datum.label)}
        />
      </div>
    </section>
  {/each}
  <div role="status" class="sr-only">{reviewLoading ? t('common.ui.loading') : ''}</div>
  {#if review}
    <section
      class="rounded-xl border border-[var(--color-primary)]/40 p-4 space-y-3"
      aria-busy={reviewLoading}
    >
      <div class="flex items-center justify-between gap-3">
        <h4 class="font-semibold">
          {t('analytics.confidenceAnalysis.review')} · {review.row.model ||
            t('analytics.confidenceAnalysis.unknownModel')} · {(review.bin * 100) /
            CONFIDENCE_BINS}–{((review.bin + 1) * 100) / CONFIDENCE_BINS}%
        </h4>
        <button type="button" onclick={() => (review = null)}>{t('common.buttons.close')}</button>
      </div>
      <p class="text-sm">
        {t('analytics.confidenceAnalysis.sample', {
          count: formatNumber(review.row.bins[review.bin] ?? 0),
        })}
      </p>
      {#if reviewLoading}<p>{t('common.ui.loading')}</p>
      {:else if reviewError}<p role="alert">{t('analytics.errors.loadFailed')}</p>
      {:else}
        <ul class="divide-y divide-[var(--color-base-200)]">
          {#each reviewRows as detection (detection.id)}
            <li class="py-2 flex justify-between gap-4 text-sm">
              <a
                class="text-[var(--color-primary)] underline"
                href={buildAppUrl(`/ui/detections/${detection.id}`)}
                >{localizeSpeciesName(
                  detection.species,
                  detection.commonName || detection.species
                )}</a
              >
              <span
                >{formatDateTime(new Date(detection.detectedAt * 1000))} · {(
                  detection.confidence * 100
                ).toFixed(1)}%</span
              >
            </li>
          {:else}<li>{t('analytics.charts.noDataAvailable')}</li>{/each}
        </ul>
        <div class="flex gap-4">
          {#if review.offset > 0}<button
              type="button"
              onclick={() => {
                if (review)
                  review = { ...review, offset: Math.max(0, review.offset - REVIEW_PAGE_SIZE) };
              }}>{t('common.buttons.previous')}</button
            >{/if}
          {#if review.offset + REVIEW_PAGE_SIZE < (review.row.bins[review.bin] ?? 0)}<button
              type="button"
              onclick={() => {
                if (review) review = { ...review, offset: review.offset + REVIEW_PAGE_SIZE };
              }}>{t('common.buttons.next')}</button
            >{/if}
        </div>
      {/if}
    </section>
  {/if}
</div>

<script lang="ts">
  import { untrack } from 'svelte';
  import { t } from '$lib/i18n';
  import { api } from '$lib/utils/api';
  import { formatDate, formatNumber } from '$lib/utils/formatters';
  import { getLogger } from '$lib/utils/logger';
  import { localizeSpeciesName } from '$lib/utils/speciesDisplay';
  import { buildAppUrl } from '$lib/utils/urlHelpers';
  import LoadingSpinner from '$lib/desktop/components/ui/LoadingSpinner.svelte';
  import Button from '$lib/desktop/components/ui/Button.svelte';
  import type { AnalyticsParams } from '../registry/types';
  import {
    comparisonWindow,
    peakActivityWindow,
    relativeChange,
    speciesBalance,
    speciesMovements,
    type ComparisonWindow,
    type PatternSpecies,
    type SpeciesBalance,
  } from '../utils/summaryPatterns';

  interface Props {
    params: AnalyticsParams;
    species: PatternSpecies[] | null;
    hourly: { hour: number; count: number }[];
    isLoading: boolean;
  }
  let { params, species, hourly, isLoading }: Props = $props();
  const logger = getLogger('summary-patterns');
  const MOVEMENT_LIMIT = 3;
  const CHANGE_EPSILON = 0.000001;
  const PERCENT_SCALE = 100;
  const signed = (value: number) => `${value > 0 ? '+' : ''}${formatNumber(value, 1)}`;
  const hourLabel = (hour: number) => `${String(hour).padStart(2, '0')}:00`;
  const name = (row: { scientific: string; name: string }) =>
    localizeSpeciesName(row.scientific, row.name);
  const balance = $derived(species === null ? null : speciesBalance(species));
  const peak = $derived(peakActivityWindow(hourly));
  const period = $derived(comparisonWindow(params.startDate, params.endDate));
  let comparison = $state<{ current: SpeciesBalance; previous: SpeciesBalance } | null>(null);
  let comparing = $state(false);
  let failed = $state(false);
  let fetchSequence = 0;
  const movements = $derived(
    comparison ? speciesMovements(comparison.current, comparison.previous) : []
  );
  const gainers = $derived(
    movements.filter(row => row.change > CHANGE_EPSILON).slice(0, MOVEMENT_LIMIT)
  );
  const decliners = $derived(
    movements.filter(row => row.change < -CHANGE_EPSILON).slice(0, MOVEMENT_LIMIT)
  );
  const detectionChange = $derived(
    comparison ? relativeChange(comparison.current.total, comparison.previous.total) : null
  );
  const diversityChange = $derived(
    comparison ? relativeChange(comparison.current.effective, comparison.previous.effective) : null
  );
  const retained = $derived.by(() => {
    if (!comparison || !comparison.previous.species.length) return null;
    const currentKeys = new Set(comparison.current.species.map(row => row.key));
    return (
      (comparison.previous.species.filter(row => currentKeys.has(row.key)).length /
        comparison.previous.species.length) *
      PERCENT_SCALE
    );
  });

  async function fetchPeriod(start: string, end: string) {
    const query = new URLSearchParams({ start_date: start, end_date: end });
    const rows = await api.get<PatternSpecies[]>(`/api/v2/analytics/species/summary?${query}`);
    if (!Array.isArray(rows)) throw new Error('Invalid species comparison response');
    return speciesBalance(rows);
  }
  async function loadComparison(window: ComparisonWindow, sequence: number) {
    try {
      const [current, previous] = await Promise.all([
        fetchPeriod(window.currentStart, window.currentEnd),
        fetchPeriod(window.previousStart, window.previousEnd),
      ]);
      if (sequence !== fetchSequence) return;
      comparison = { current, previous };
    } catch (error) {
      if (sequence !== fetchSequence) return;
      failed = true;
      logger.error('Unable to compare Summary periods', error);
    } finally {
      if (sequence === fetchSequence) comparing = false;
    }
  }
  $effect(() => {
    const window = period;
    const sequence = ++fetchSequence;
    comparison = null;
    failed = false;
    comparing = window !== null;
    if (window) untrack(() => void loadComparison(window, sequence));
    return () => {
      fetchSequence++;
    };
  });
  function retry() {
    if (!period) return;
    failed = false;
    comparing = true;
    void loadComparison(period, ++fetchSequence);
  }
  function speciesUrl(scientific: string, common: string) {
    const query = new URLSearchParams({
      queryType: 'search',
      species: scientific || common,
      start_date: period?.currentStart ?? '',
      end_date: period?.currentEnd ?? '',
    });
    return buildAppUrl(`/ui/detections?${query}`);
  }
</script>

<section
  class="card min-w-0 bg-[var(--color-base-100)] shadow-xs"
  aria-label={t('analytics.patterns.balanceTitle')}
>
  <div class="card-body gap-5 p-4 md:p-6">
    <div>
      <h2 class="card-title">{t('analytics.patterns.balanceTitle')}</h2>
      <p class="text-sm opacity-70">{t('analytics.patterns.balanceHelp')}</p>
    </div>
    {#if isLoading}
      <div class="flex justify-center p-8"><LoadingSpinner size="lg" /></div>
    {:else if balance?.total}
      <dl class="grid grid-cols-2 gap-2 lg:grid-cols-4">
        <div class="rounded-lg bg-[var(--color-base-200)]/55 px-4 py-3">
          <dt class="text-xs font-medium uppercase tracking-wide opacity-65">
            {t('analytics.patterns.effective')}
          </dt>
          <dd class="mt-1 text-xl font-semibold tabular-nums">
            {formatNumber(balance.effective, 1)}
          </dd>
        </div>
        <div class="rounded-lg bg-[var(--color-base-200)]/55 px-4 py-3">
          <dt class="text-xs font-medium uppercase tracking-wide opacity-65">
            {t('analytics.patterns.topFive')}
          </dt>
          <dd class="mt-1 text-xl font-semibold tabular-nums">
            {formatNumber(balance.topFiveShare, 1)}%
          </dd>
        </div>
        <div class="rounded-lg bg-[var(--color-base-200)]/55 px-4 py-3">
          <dt class="text-xs font-medium uppercase tracking-wide opacity-65">
            {t('analytics.patterns.coverage')}
          </dt>
          <dd class="mt-1 flex items-baseline gap-1">
            <span class="text-xl font-semibold tabular-nums"
              >{formatNumber(balance.speciesForCoverage)}</span
            ><span class="text-xs opacity-65">/ {formatNumber(balance.species.length)}</span>
          </dd>
        </div>
        <div class="rounded-lg bg-[var(--color-base-200)]/55 px-4 py-3">
          <dt class="text-xs font-medium uppercase tracking-wide opacity-65">
            {t('analytics.patterns.peakWindow')}
          </dt>
          <dd class="mt-1 flex flex-wrap items-baseline gap-1">
            <span class="text-xl font-semibold tabular-nums"
              >{peak ? `${hourLabel(peak.start)}–${hourLabel(peak.end)}` : '—'}</span
            >{#if peak}<span class="text-xs opacity-65"
                >{t('analytics.patterns.windowShare', {
                  percentage: formatNumber(peak.share, 1),
                })}</span
              >{/if}
          </dd>
        </div>
      </dl>
    {:else}<p class="py-4 text-sm opacity-70">{t('analytics.charts.noDataAvailable')}</p>{/if}

    <div class="border-t border-[var(--color-base-200)] pt-5" aria-busy={comparing}>
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 class="text-base font-semibold">{t('analytics.patterns.comparisonTitle')}</h3>
          {#if period}<p class="mt-1 text-sm opacity-70">
              {t('analytics.patterns.comparisonHelp')}
            </p>{/if}
        </div>
        {#if period}<p
            class="rounded-md bg-[var(--color-base-200)]/55 px-3 py-2 text-xs opacity-75"
          >
            {formatDate(period.currentStart)} – {formatDate(period.currentEnd)}
            <span class="mx-1">{t('analytics.patterns.versus')}</span>
            {formatDate(period.previousStart)} – {formatDate(period.previousEnd)}
          </p>{/if}
      </div>
      {#if !period}
        <p class="mt-3 text-sm opacity-70">{t('analytics.patterns.completedOnly')}</p>
      {:else if comparing}
        <div class="flex justify-center p-8"><LoadingSpinner size="lg" /></div>
      {:else if failed}
        <p role="alert" class="mt-3 text-sm">{t('analytics.patterns.comparisonError')}</p>
        <Button variant="ghost" onclick={retry}>{t('common.retry')}</Button>
      {:else if comparison}
        <div class="mt-4 grid gap-5 lg:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)]">
          <dl class="grid grid-cols-3 gap-2 self-start">
            <div class="rounded-lg border border-[var(--color-base-200)] p-3">
              <dt class="text-sm opacity-70">{t('analytics.patterns.volumeChange')}</dt>
              <dd class="mt-1 text-xl font-semibold tabular-nums">
                {detectionChange === null ? '—' : `${signed(detectionChange)}%`}
              </dd>
            </div>
            <div class="rounded-lg border border-[var(--color-base-200)] p-3">
              <dt class="text-sm opacity-70">{t('analytics.patterns.diversityChange')}</dt>
              <dd class="mt-1 text-xl font-semibold tabular-nums">
                {diversityChange === null ? '—' : `${signed(diversityChange)}%`}
              </dd>
            </div>
            <div class="rounded-lg border border-[var(--color-base-200)] p-3">
              <dt class="text-sm opacity-70">{t('analytics.patterns.retained')}</dt>
              <dd class="mt-1 text-xl font-semibold tabular-nums">
                {retained === null ? '—' : `${formatNumber(retained, 1)}%`}
              </dd>
            </div>
          </dl>
          <div class="grid gap-4 md:grid-cols-2">
            {#each [{ title: t('analytics.patterns.gaining'), rows: gainers }, { title: t('analytics.patterns.losing'), rows: decliners }] as group (group.title)}
              <div>
                <h4 class="mb-2 text-sm font-semibold">{group.title}</h4>
                {#if group.rows.length}<ul class="space-y-2">
                    {#each group.rows as row (row.species.key)}<li
                        class="flex items-center justify-between gap-3 text-sm"
                      >
                        <a
                          class="min-w-0 truncate hover:underline"
                          href={speciesUrl(row.species.scientific, row.species.name)}
                          >{name(row.species)}</a
                        ><span class="shrink-0 tabular-nums"
                          ><strong>{signed(row.change)}</strong> pp</span
                        >
                      </li>{/each}
                  </ul>{:else}<p class="text-sm opacity-60">—</p>{/if}
              </div>
            {/each}
          </div>
        </div>
        {#if comparison.previous.total === 0}<p class="mt-4 text-sm opacity-70">
            {t('analytics.patterns.noBaseline')}
          </p>
          <a
            class="text-sm text-[var(--color-primary)] underline"
            href={buildAppUrl('/ui/analytics/summary?range=week')}
            >{t('analytics.periods.lastWeek')}</a
          >{/if}
        <p class="mt-3 text-xs opacity-70">{t('analytics.patterns.retainedHelp')}</p>
      {/if}
    </div>
  </div>
</section>

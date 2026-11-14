<!-- Summary overview: date-filtered station totals, ranked species, and calendar activity. -->
<script lang="ts">
  import { onMount, untrack, type Snippet } from 'svelte';
  import { Bird, ListChecks, TrendingUp, XCircle } from '@lucide/svelte';

  import { t, type TranslationKey } from '$lib/i18n';
  import { api } from '$lib/utils/api';
  import { generateId } from '$lib/utils/uuid';
  import { getStoredValue, setStoredValue } from '$lib/utils/storage';
  import {
    summarizeActivity,
    summarizeSystemUptime,
    type PeriodUptime,
    DEFAULT_TOP_SPECIES_LIMIT,
    MAX_TOP_SPECIES_LIMIT,
  } from '../utils/summaryMeasures';
  import { buildAppUrl } from '$lib/utils/urlHelpers';
  import { formatNumber, formatDate } from '$lib/utils/formatters';
  import { parseLocalDateString } from '$lib/utils/date';
  import { getLogger } from '$lib/utils/logger';
  import { localizeSpeciesName } from '$lib/utils/speciesDisplay';
  import LoadingSpinner from '$lib/desktop/components/ui/LoadingSpinner.svelte';

  import StatCard from './ui/StatCard.svelte';
  import BarChart from './charts/d3/BarChart.svelte';
  import PieChart, { type PieChartDatum } from './charts/d3/PieChart.svelte';
  import LineChart from './charts/d3/LineChart.svelte';
  import NewSpeciesTimelineChart from './charts/d3/NewSpeciesTimelineChart.svelte';
  import SummaryPatterns from './SummaryPatterns.svelte';
  import { aggregateTrendPoints, mapNewSpecies } from '../utils/analyticsTransforms';
  import { formatDateForAPI } from '../registry/analyticsParams';
  import type { AnalyticsParams, DateRangePreset } from '../registry/types';

  interface Props {
    /** Shared hub params; only the resolved date range drives this tab. */
    params: AnalyticsParams;
  }

  let { params }: Props = $props();

  const logger = getLogger('analytics-overview');
  const SPECIES_LIMIT_STORAGE_KEY = 'analytics.summary.speciesLimit';
  const SPECIES_STYLE_STORAGE_KEY = 'analytics.summary.speciesChartStyle';
  const LEGACY_TOP_SPECIES_LIMIT = 25;
  const speciesLimitId = generateId('summary-species-limit');
  const speciesStyleId = generateId('summary-species-style');
  const DEFAULT_SPECIES_CHART_HEIGHT = 400;
  const EXPANDED_SPECIES_CHART_HEIGHT = 700;
  const HOURS_PER_DAY = 24;
  const UPTIME_REFRESH_INTERVAL_MS = 600_000;
  const UPTIME_DISPLAY_PRECISION = 10;
  const PERCENT_SCALE = 100;
  const restoredSpeciesLimit = getStoredValue<number>(
    SPECIES_LIMIT_STORAGE_KEY,
    DEFAULT_TOP_SPECIES_LIMIT,
    value =>
      value === DEFAULT_TOP_SPECIES_LIMIT ||
      value === MAX_TOP_SPECIES_LIMIT ||
      value === LEGACY_TOP_SPECIES_LIMIT
  );
  let topSpeciesLimit = $state(
    restoredSpeciesLimit === LEGACY_TOP_SPECIES_LIMIT ? MAX_TOP_SPECIES_LIMIT : restoredSpeciesLimit
  );
  let speciesChartStyle = $state(
    getStoredValue<'bar' | 'pie'>(
      SPECIES_STYLE_STORAGE_KEY,
      'bar',
      value => value === 'bar' || value === 'pie'
    )
  );
  const showSpeciesPie = $derived(
    topSpeciesLimit === DEFAULT_TOP_SPECIES_LIMIT && speciesChartStyle === 'pie'
  );
  const speciesChartHeight = $derived(
    topSpeciesLimit === MAX_TOP_SPECIES_LIMIT
      ? EXPANDED_SPECIES_CHART_HEIGHT
      : DEFAULT_SPECIES_CHART_HEIGHT
  );

  const NEW_SPECIES_PAGE_SIZE = 100;
  const NEW_SPECIES_ROW_HEIGHT = 28;
  const NEW_SPECIES_CHART_PADDING = 105;
  const NEW_SPECIES_MIN_HEIGHT = 384;

  // Period subtitle for the StatCards, derived from the shared range preset.
  // Reuses the existing analytics.periods.* strings; `quarter` (90 days) maps to
  // the legacy "Last 90 Days" label.
  const RANGE_LABEL_KEYS: Record<DateRangePreset, TranslationKey> = {
    today: 'analytics.periods.today',
    threeDays: 'analytics.periods.last3Days',
    week: 'analytics.periods.lastWeek',
    month: 'analytics.periods.lastMonth',
    quarter: 'analytics.periods.last90Days',
    year: 'analytics.periods.lastYear',
    custom: 'analytics.periods.customRange',
  };

  // Type definitions
  interface Summary {
    totalDetections: number;
    avgConfidence: number | null;
    uniqueSpecies: number;
    mostCommonSpecies: string;
    // Canonical scientific name of the most-common species, so the displayed
    // common name can localize per visitor while the lookup stays canonical.
    mostCommonScientific: string;
    mostCommonCount: number;
  }

  interface SpeciesData {
    common_name: string;
    scientific_name?: string;
    count: number;
    avg_confidence: number;
  }

  interface HourlyData {
    hour: number;
    count: number;
  }

  interface TrendData {
    data: {
      date: string;
      count: number;
    }[];
  }

  interface NewSpeciesData {
    common_name: string;
    scientific_name: string;
    first_heard_date: string;
  }

  interface ChartData {
    species: SpeciesData[];
    hourly: HourlyData[];
    trend: TrendData | null;
    newSpecies: NewSpeciesData[];
  }

  // State variables
  let isLoading = $state<boolean>(true);
  let error = $state<string | null>(null);
  let fullSpecies = $state<SpeciesData[] | null>(null);

  // Summary data
  let summary = $state<Summary>({
    totalDetections: 0,
    avgConfidence: null,
    uniqueSpecies: 0,
    mostCommonSpecies: '',
    mostCommonScientific: '',
    mostCommonCount: 0,
  });

  // Data arrays

  // Chart data storage
  let chartData = $state<ChartData>({
    species: [],
    hourly: [],
    trend: null,
    newSpecies: [],
  });

  let uptime = $state<PeriodUptime | null>(null);

  onMount(() => {
    if (restoredSpeciesLimit === LEGACY_TOP_SPECIES_LIMIT) {
      setStoredValue(SPECIES_LIMIT_STORAGE_KEY, MAX_TOP_SPECIES_LIMIT);
    }
    const timer = setInterval(() => {
      void fetchUptime(analyticsFetchSeq, params.startDate, params.endDate);
    }, UPTIME_REFRESH_INTERVAL_MS);
    return () => clearInterval(timer);
  });

  async function fetchUptime(seq: number, start: Date, end: Date) {
    try {
      const info = await api.get<{
        uptime_seconds: number;
        app_start_time: string;
        app_uptime_seconds: number;
      }>('/api/v2/system/info');
      if (seq !== analyticsFetchSeq) return;
      uptime = summarizeSystemUptime(
        info.uptime_seconds,
        info.app_start_time,
        info.app_uptime_seconds,
        start,
        end
      );
    } catch (err) {
      if (seq !== analyticsFetchSeq) return;
      logger.error('Error fetching Summary uptime:', err);
      uptime = null;
    }
  }

  // Monotonic token guarding against stale-response races. Each fetchData() run
  // captures the latest value; a fetcher only commits its result when its captured
  // token still matches, so a slower earlier request can no longer overwrite the
  // data from a newer one after rapid range changes. Non-reactive on purpose.
  let analyticsFetchSeq = 0;

  // Derived chart inputs built from the reactive chartData via pure transforms.
  // Species distribution: sorted desc by count, mapped to labelled bars with
  // per-species colors from the D3 theme palette (applied inside BarChart).
  const speciesBars = $derived(
    [...(chartData.species ?? [])]
      .sort((a, b) => b.count - a.count)
      .slice(0, topSpeciesLimit)
      .map(s => ({ label: localizeSpeciesName(s.scientific_name, s.common_name), value: s.count }))
  );

  const shownSpeciesCount = $derived(speciesBars.reduce((sum, species) => sum + species.value, 0));
  const speciesPieData = $derived.by(() => {
    const rows: PieChartDatum[] = [...speciesBars];
    const otherDetections = summary.totalDetections - shownSpeciesCount;
    if (otherDetections > 0) {
      rows.push({
        label: t('analytics.patterns.otherSpecies'),
        value: otherDetections,
        clickable: false,
      });
    }
    return rows;
  });
  const speciesShare = $derived(
    summary.totalDetections > 0 && shownSpeciesCount <= summary.totalDetections
      ? (shownSpeciesCount / summary.totalDetections) * 100
      : null
  );
  const activity = $derived(summarizeActivity(chartData.trend, params.startDate, params.endDate));
  const hourlyBars = $derived(
    Array.from({ length: HOURS_PER_DAY }, (_, hour) => ({
      label: String(hour).padStart(2, '0'),
      value: chartData.hourly
        .filter(row => row.hour === hour && Number.isFinite(row.count) && row.count >= 0)
        .reduce((total, row) => total + row.count, 0),
    }))
  );

  function openSpecies(label: string) {
    const species = chartData.species.find(
      row => localizeSpeciesName(row.scientific_name, row.common_name) === label
    );
    if (!species) return;
    const search = new URLSearchParams({
      queryType: 'search',
      species: species.scientific_name || species.common_name,
      start_date: formatDateForAPI(params.startDate),
      end_date: formatDateForAPI(params.endDate),
    });
    window.location.href = buildAppUrl(`/ui/detections?${search}`);
  }

  // Detection trend: aggregated/sorted daily points, wrapped as a single series.
  const trendSeries = $derived([
    {
      id: 'daily',
      label: t('analytics.charts.dailyDetections'),
      data: aggregateTrendPoints(chartData.trend),
    },
  ]);

  // New species: API rows mapped to { commonName, scientificName, firstHeard }.
  // Keep every species in the selected period; a scrollable chart preserves legible rows.
  const newSpeciesPoints = $derived(
    mapNewSpecies(chartData.newSpecies).sort(
      (a, b) => b.firstHeard.getTime() - a.firstHeard.getTime()
    )
  );
  const newSpeciesChartHeight = $derived(
    Math.max(
      NEW_SPECIES_MIN_HEIGHT,
      newSpeciesPoints.length * NEW_SPECIES_ROW_HEIGHT + NEW_SPECIES_CHART_PADDING
    )
  );

  // Daily API points are parsed at local noon. Match those calendar boundaries
  // so the final day's point is inside the domain even before noon or for custom dates.
  const chartDateRange = $derived<[Date, Date]>([
    parseLocalDateString(formatDateForAPI(params.startDate)) ?? params.startDate,
    parseLocalDateString(formatDateForAPI(params.endDate)) ?? params.endDate,
  ]);

  // Subtitle shown on every StatCard, describing the active range.
  const periodLabel = $derived(t(RANGE_LABEL_KEYS[params.range]));

  // Refetch whenever the resolved date range changes. Day-granular so a same-day
  // re-resolve of the Date objects does not spuriously refetch. The first run
  // (on mount) performs the initial load, so no separate onMount is needed.
  const rangeKey = $derived(
    `${formatDateForAPI(params.startDate)}|${formatDateForAPI(params.endDate)}`
  );
  $effect(() => {
    void rangeKey;
    untrack(() => fetchData());
  });

  // Fetch all data for the current range.
  async function fetchData() {
    const snapshot = untrack(() => params);
    const startDate = formatDateForAPI(snapshot.startDate);
    const endDate = formatDateForAPI(snapshot.endDate);

    const seq = ++analyticsFetchSeq;
    isLoading = true;
    error = null;
    fullSpecies = null;
    uptime = null;
    void fetchUptime(seq, snapshot.startDate, snapshot.endDate);

    // Clear the previous range's data up front so a slow or failing request can
    // never leave stale metrics on screen (the loading overlays mask the blank
    // slate). Each fetcher then only commits success; failures just log + rethrow.
    summary = {
      totalDetections: 0,
      avgConfidence: null,
      uniqueSpecies: 0,
      mostCommonSpecies: '',
      mostCommonScientific: '',
      mostCommonCount: 0,
    };
    chartData = { species: [], hourly: [], trend: null, newSpecies: [] };

    logger.debug('Loading overview analytics', { startDate, endDate });

    // Each fetcher commits only for the latest run (the sequence-token guard)
    // and rethrows on failure after logging, so allSettled lets us tell whether
    // the whole load failed. Only the most recent run owns the shared loading /
    // error state; a superseded run bails before touching it.
    const results = await Promise.allSettled([
      fetchSummaryData(seq, startDate, endDate),
      fetchSpeciesSummary(seq, startDate, endDate),
      fetchHourlyData(seq, startDate, endDate),
      fetchTrendData(seq, startDate, endDate),
      fetchNewSpeciesData(seq, startDate, endDate),
    ]);

    if (seq !== analyticsFetchSeq) return;

    // Surface a global error only when every request failed; a partial failure
    // is communicated by the affected chart's own empty state, so a full-page
    // error banner over otherwise-loaded data would be misleading.
    if (results.every(result => result.status === 'rejected')) {
      error = t('analytics.loadingError');
    }
    isLoading = false;
  }

  // Fetch summary metrics
  async function fetchSummaryData(seq: number, startDate: string, endDate: string) {
    try {
      const search = new URLSearchParams();
      if (startDate) search.set('start_date', startDate);
      if (endDate) search.set('end_date', endDate);

      const url = `/api/v2/analytics/species/summary?${search}`;
      const speciesData = await api.get<SpeciesData[]>(url);
      if (seq !== analyticsFetchSeq) return; // superseded by a newer fetch
      const speciesArray = Array.isArray(speciesData) ? speciesData : [];
      fullSpecies = speciesArray;

      // Calculate summary metrics
      let totalDetections = 0;
      let confidenceSum = 0;
      let confidenceCount = 0;
      let mostCommonSpecies = '';
      let mostCommonScientific = '';
      let mostCommonCount = 0;

      speciesArray.forEach(species => {
        const count = species.count || 0;

        totalDetections += count;
        if (
          count > 0 &&
          Number.isFinite(species.avg_confidence) &&
          species.avg_confidence >= 0 &&
          species.avg_confidence <= 1
        ) {
          confidenceSum += species.avg_confidence * count;
          confidenceCount += count;
        }

        if (count > mostCommonCount) {
          mostCommonCount = count;
          mostCommonSpecies = species.common_name || t('analytics.recentDetections.unknown');
          mostCommonScientific = species.scientific_name || '';
        }
      });

      summary = {
        totalDetections,
        avgConfidence:
          confidenceCount > 0 && confidenceCount === totalDetections
            ? confidenceSum / confidenceCount
            : null,
        uniqueSpecies: speciesArray.length,
        mostCommonSpecies,
        mostCommonScientific,
        mostCommonCount,
      };
    } catch (err) {
      if (seq !== analyticsFetchSeq) return; // superseded by a newer fetch
      logger.error('Error fetching summary data:', err);
      throw err; // rethrow so fetchData can detect a total-failure load
    }
  }

  // Fetch species summary for chart
  async function fetchSpeciesSummary(seq: number, startDate: string, endDate: string) {
    try {
      const search = new URLSearchParams({ limit: String(MAX_TOP_SPECIES_LIMIT) });
      if (startDate) search.set('start_date', startDate);
      if (endDate) search.set('end_date', endDate);

      const url = `/api/v2/analytics/species/summary?${search}`;
      const speciesData = await api.get<SpeciesData[]>(url);
      if (seq !== analyticsFetchSeq) return; // superseded by a newer fetch
      chartData.species = Array.isArray(speciesData) ? speciesData : [];
    } catch (err) {
      if (seq !== analyticsFetchSeq) return; // superseded by a newer fetch
      logger.error('Error fetching species summary:', err);
      throw err; // rethrow so fetchData can detect a total-failure load
    }
  }

  // Fetch hourly counts for the selected date range
  async function fetchHourlyData(seq: number, startDate: string, endDate: string) {
    try {
      const search = new URLSearchParams();
      if (startDate) search.set('start_date', startDate);
      if (endDate) search.set('end_date', endDate);

      const url = `/api/v2/analytics/time/distribution/hourly?${search}`;
      const timeData = await api.get<HourlyData[]>(url);
      if (seq !== analyticsFetchSeq) return; // superseded by a newer fetch
      chartData.hourly = Array.isArray(timeData) ? timeData : [];
    } catch (err) {
      if (seq !== analyticsFetchSeq) return; // superseded by a newer fetch
      logger.error('Error fetching hourly data:', err);
      throw err; // rethrow so fetchData can detect a total-failure load
    }
  }

  // Fetch trend data
  async function fetchTrendData(seq: number, startDate: string, endDate: string) {
    try {
      const search = new URLSearchParams();

      // The daily analytics endpoint requires a start_date parameter; the shared
      // range always resolves one, but keep the legacy 30-day fallback for safety.
      if (startDate) {
        search.set('start_date', startDate);
      } else {
        const defaultStart = new Date();
        defaultStart.setDate(defaultStart.getDate() - 30);
        search.set('start_date', formatDateForAPI(defaultStart));
      }

      if (endDate) search.set('end_date', endDate);

      const url = `/api/v2/analytics/time/daily?${search}`;
      const trendData = await api.get<TrendData>(url);
      if (seq !== analyticsFetchSeq) return; // superseded by a newer fetch
      chartData.trend = trendData ?? { data: [] };
    } catch (err) {
      if (seq !== analyticsFetchSeq) return; // superseded by a newer fetch
      logger.error('Error fetching trend data:', err);
      throw err; // rethrow so fetchData can detect a total-failure load
    }
  }

  // Fetch new species data
  async function fetchNewSpeciesData(seq: number, startDate: string, endDate: string) {
    try {
      const search = new URLSearchParams();
      if (startDate) search.set('start_date', startDate);
      if (endDate) search.set('end_date', endDate);

      search.set('limit', String(NEW_SPECIES_PAGE_SIZE));
      const rows: NewSpeciesData[] = [];
      for (let offset = 0; ; offset += NEW_SPECIES_PAGE_SIZE) {
        search.set('offset', String(offset));
        const data = await api.get<NewSpeciesData[]>(
          `/api/v2/analytics/species/detections/new?${search}`
        );
        if (seq !== analyticsFetchSeq) return;
        if (!Array.isArray(data)) throw new Error('Invalid new-species response');
        rows.push(...data);
        if (data.length < NEW_SPECIES_PAGE_SIZE) break;
      }
      chartData.newSpecies = rows;
    } catch (err) {
      if (seq !== analyticsFetchSeq) return; // superseded by a newer fetch
      logger.error('Error fetching new species data:', err);
      throw err; // rethrow so fetchData can detect a total-failure load
    }
  }
</script>

<div class="space-y-4">
  {#if error}
    <div class="alert alert-error">
      <XCircle class="size-6" />
      <span>{error}</span>
    </div>
  {/if}

  <!-- Summary Stats Cards -->
  <div class="grid gap-4 summary-cards-grid">
    <!-- Total Detections Card -->
    <StatCard
      title={t('analytics.stats.totalDetections')}
      value={formatNumber(summary.totalDetections)}
      subtitle={periodLabel}
      iconClassName="bg-[var(--color-primary)]/20"
      {isLoading}
    >
      {#snippet icon()}<Bird class="h-6 w-6 text-[var(--color-primary)]" />{/snippet}
    </StatCard>

    <!-- Unique Species Card -->
    <StatCard
      title={t('analytics.stats.uniqueSpecies')}
      value={formatNumber(summary.uniqueSpecies)}
      secondaryValue={`${formatNumber(newSpeciesPoints.length)} ${t('species.tracking.newSpecies')}`}
      subtitle={periodLabel}
      iconClassName="bg-[var(--color-secondary)]/20"
      {isLoading}
    >
      {#snippet icon()}<ListChecks class="h-6 w-6 text-[var(--color-primary)]" />{/snippet}
    </StatCard>

    <!-- Most Common Species Card -->
    <StatCard
      title={t('analytics.stats.mostCommon')}
      value={summary.mostCommonCount > 0
        ? localizeSpeciesName(summary.mostCommonScientific, summary.mostCommonSpecies)
        : t('analytics.stats.none')}
      subtitle={summary.mostCommonCount > 0
        ? formatNumber(summary.mostCommonCount) + ' ' + t('analytics.stats.detections')
        : ''}
      iconClassName="bg-[var(--color-success)]/20"
      valueClassName="text-lg leading-tight break-words"
      {isLoading}
    >
      {#snippet icon()}<TrendingUp class="h-6 w-6 text-[var(--color-primary)]" />{/snippet}
    </StatCard>
  </div>

  <section
    class="activity-snapshot rounded-xl border border-[var(--color-base-200)] bg-[var(--color-base-100)] p-4 md:p-6 space-y-4"
  >
    <h2 class="text-lg font-semibold">{t('analytics.summary.activityTitle')}</h2>
    <div class="grid gap-4 summary-cards-grid activity-cards-grid">
      <StatCard
        title={t('analytics.stats.avgConfidence')}
        value={summary.avgConfidence === null
          ? '—'
          : `${formatNumber(summary.avgConfidence * PERCENT_SCALE, 1)}%`}
        subtitle={periodLabel}
        {isLoading}
      />
      <StatCard
        title={t('analytics.summary.uptime')}
        value={uptime === null
          ? t('analytics.summary.uptimeUnavailable')
          : uptime.complete
            ? `${formatNumber(uptime.percentage, 1)}%`
            : `${formatNumber(
                Math.floor(uptime.percentage * UPTIME_DISPLAY_PRECISION) / UPTIME_DISPLAY_PRECISION,
                1
              )}%`}
        subtitle={uptime === null
          ? t('analytics.summary.uptimeHistoryMissing')
          : uptime.complete
            ? t('analytics.summary.uptimeScope')
            : undefined}
        {isLoading}
      />
      <StatCard
        title={t('analytics.summary.calendarAverage')}
        value={activity ? formatNumber(activity.dailyAverage, 1) : '—'}
        subtitle={periodLabel}
        {isLoading}
      />
      <StatCard
        title={t('analytics.summary.busiestDay')}
        value={activity?.busiest ? formatDate(activity.busiest.date) : '—'}
        subtitle={activity?.busiest
          ? t('analytics.confidenceAnalysis.sample', {
              count: formatNumber(activity.busiest.count),
            })
          : t('analytics.charts.noDataAvailable')}
        {isLoading}
      />
    </div>
  </section>

  <!-- Charts Section -->
  <div class="grid gap-4 charts-grid">
    <div class:species-expanded={topSpeciesLimit === MAX_TOP_SPECIES_LIMIT} class="min-w-0">
      {@render chartCard(
        t('analytics.summary.topSpecies', { count: topSpeciesLimit }),
        '',
        speciesChart,
        speciesBars.length === 0,
        t('analytics.charts.noDataAvailable'),
        speciesControls,
        speciesChartHeight
      )}
    </div>

    <div class:species-expanded={topSpeciesLimit === MAX_TOP_SPECIES_LIMIT} class="min-w-0">
      {@render chartCard(
        t('analytics.summary.hourlyTitle'),
        'h-96',
        hourlyChart,
        hourlyBars.every(bar => bar.value === 0),
        t('analytics.charts.noDataAvailable')
      )}
    </div>
  </div>

  <!-- Trend Chart -->
  {@render chartCard(
    t('analytics.charts.detectionTrends'),
    'h-80',
    trendChart,
    trendSeries[0].data.length === 0,
    t('analytics.charts.noDataAvailable')
  )}

  <SummaryPatterns {params} species={fullSpecies} hourly={chartData.hourly} {isLoading} />

  <!-- New Species Chart -->
  {@render chartCard(
    t('analytics.charts.newSpeciesDetected'),
    'h-96',
    newSpeciesChart,
    newSpeciesPoints.length === 0,
    t('analytics.charts.noNewSpecies')
  )}
</div>

<!-- Shared card chrome for the overview charts (loading overlay + empty state). -->
{#snippet chartCard(
  title: string,
  chartHeight: string,
  chart: Snippet,
  empty: boolean,
  emptyMessage: string,
  controls?: Snippet,
  height?: number
)}
  <div class="card bg-[var(--color-base-100)] shadow-xs">
    <div class="card-body p-4 md:p-6">
      <div class="flex flex-wrap items-center justify-between gap-3 mb-3">
        <h2 class="card-title">{title}</h2>
        {#if controls}{@render controls()}{/if}
      </div>
      {#if empty && !isLoading}
        <div class="text-center py-4 text-[var(--color-base-content)] opacity-50">
          {emptyMessage}
        </div>
      {:else}
        <div class="relative" aria-busy={isLoading}>
          <div
            class="chart-container {chartHeight}"
            style:height={height === undefined ? undefined : `${height}px`}
            class:invisible={isLoading}
          >
            {@render chart()}
          </div>
          {#if isLoading}
            <div class="absolute inset-0 flex justify-center items-center">
              <LoadingSpinner size="lg" />
            </div>
          {/if}
        </div>
      {/if}
    </div>
  </div>
{/snippet}

{#snippet speciesControls()}
  <div class="flex flex-wrap items-center gap-3 text-sm">
    <div class="flex items-center gap-2">
      <label for={speciesLimitId}>{t('analytics.summary.speciesLimit')}</label>
      <select
        id={speciesLimitId}
        value={topSpeciesLimit}
        class="rounded-lg border border-[var(--color-base-300)] bg-[var(--color-base-100)] px-3 py-2"
        onchange={event => {
          const limit = Number(event.currentTarget.value);
          if (limit === DEFAULT_TOP_SPECIES_LIMIT || limit === MAX_TOP_SPECIES_LIMIT) {
            topSpeciesLimit = limit;
            setStoredValue(SPECIES_LIMIT_STORAGE_KEY, limit);
          }
        }}
      >
        <option value={DEFAULT_TOP_SPECIES_LIMIT}
          >{t('analytics.summary.topSpecies', { count: DEFAULT_TOP_SPECIES_LIMIT })}</option
        >
        <option value={MAX_TOP_SPECIES_LIMIT}
          >{t('analytics.summary.topSpecies', { count: MAX_TOP_SPECIES_LIMIT })}</option
        >
      </select>
    </div>
    {#if topSpeciesLimit === DEFAULT_TOP_SPECIES_LIMIT}
      <div class="flex items-center gap-2">
        <label for={speciesStyleId}>{t('analytics.summary.chartStyle')}</label>
        <select
          id={speciesStyleId}
          value={speciesChartStyle}
          class="rounded-lg border border-[var(--color-base-300)] bg-[var(--color-base-100)] px-3 py-2"
          onchange={event => {
            const style = event.currentTarget.value;
            if (style === 'bar' || style === 'pie') {
              speciesChartStyle = style;
              setStoredValue(SPECIES_STYLE_STORAGE_KEY, style);
            }
          }}
        >
          <option value="bar">{t('analytics.summary.barChart')}</option>
          <option value="pie">{t('analytics.summary.pieChart')}</option>
        </select>
      </div>
    {/if}
  </div>
  {#if speciesShare !== null && !isLoading}
    <p class="text-xs opacity-70">
      {t('analytics.summary.coverage')}: {formatNumber(shownSpeciesCount)} / {formatNumber(
        summary.totalDetections
      )} ({speciesShare.toFixed(1)}%)
    </p>
  {/if}
{/snippet}

{#snippet hourlyChart()}
  <BarChart
    data={hourlyBars}
    height={384}
    categoryAxisLabel={t('analytics.advanced.charts.nocturnal.axisHour')}
    valueAxisLabel={t('analytics.charts.numberOfDetections')}
    valueTooltipLabel={t('analytics.charts.detections')}
    formatValue={formatNumber}
    ariaLabel={t('analytics.summary.hourlyTitle')}
  />
{/snippet}

{#snippet speciesChart()}
  {#if showSpeciesPie}
    <PieChart
      data={speciesPieData}
      height={speciesChartHeight}
      formatValue={formatNumber}
      formatPercentage={value => formatNumber(value, 1)}
      valueTooltipLabel={t('analytics.charts.detections')}
      ariaLabel={t('analytics.summary.topSpecies', { count: topSpeciesLimit })}
      onSliceClick={datum => openSpecies(datum.label)}
    />
  {:else}
    <BarChart
      data={speciesBars}
      height={speciesChartHeight}
      orientation="horizontal"
      valueAxisLabel={t('analytics.charts.numberOfDetections')}
      valueTooltipLabel={t('analytics.charts.detections')}
      formatValue={formatNumber}
      ariaLabel={t('analytics.summary.topSpecies', { count: topSpeciesLimit })}
      onBarClick={datum => openSpecies(datum.label)}
    />
  {/if}
{/snippet}

{#snippet trendChart()}
  <LineChart
    series={trendSeries}
    valueAxisLabel={t('analytics.charts.numberOfDetections')}
    dateAxisLabel={t('analytics.charts.date')}
    valueTooltipLabel={t('analytics.charts.detections')}
    dateRange={chartDateRange}
    formatValue={formatNumber}
    ariaLabel={t('analytics.charts.detectionTrends')}
  />
{/snippet}

{#snippet newSpeciesChart()}
  <!-- The scrollable timeline needs keyboard focus so all species can be reached with arrow keys. -->
  <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
  <div
    class="h-full overflow-y-auto"
    tabindex="0"
    role="region"
    aria-label={t('analytics.charts.newSpeciesDetected')}
  >
    <div style:height={`${newSpeciesChartHeight}px`}>
      <NewSpeciesTimelineChart
        data={newSpeciesPoints}
        dateRange={chartDateRange}
        firstHeardLabel={t('analytics.charts.firstHeard')}
        dateAxisLabel={t('analytics.charts.firstHeardDate')}
        ariaLabel={t('analytics.charts.newSpeciesDetected')}
        height={newSpeciesChartHeight}
      />
    </div>
  </div>
{/snippet}

<style>
  .species-expanded {
    grid-column: 1 / -1;
  }

  .card-padding {
    padding: 1rem;
  }

  @media (min-width: 768px) {
    .card-padding {
      padding: 1.5rem;
    }
  }

  /* Station totals use three columns; the activity snapshot has four metrics. */
  .activity-snapshot {
    container-type: inline-size;
  }

  .summary-cards-grid {
    display: grid;
    grid-template-columns: 1fr;
  }

  .activity-cards-grid {
    grid-auto-rows: 1fr;
  }

  @media (min-width: 768px) {
    .summary-cards-grid {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }

  @media (min-width: 1024px) {
    .summary-cards-grid {
      grid-template-columns: repeat(3, minmax(0, 1fr));
    }

    .activity-cards-grid {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }

  /* Use four columns only when each card has room for full-size values and padding. */
  @container (min-width: 72rem) {
    .activity-cards-grid {
      grid-template-columns: repeat(4, minmax(0, 1fr));
    }
  }

  /* Charts grid - matches grid-cols-1 lg:grid-cols-2 */
  .charts-grid {
    display: grid;
    grid-template-columns: 1fr;
  }

  @media (min-width: 1024px) {
    .charts-grid {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }
</style>

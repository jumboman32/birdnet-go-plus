<script lang="ts">
  import { onMount } from 'svelte';
  import Button from '$lib/desktop/components/ui/Button.svelte';
  import { ChevronLeft, ChevronRight, RefreshCw, Search } from '@lucide/svelte';
  import { interpolateRgb, scaleSequential } from 'd3';
  import { getLocale, t } from '$lib/i18n';
  import { ApiError, fetchWithCSRF } from '$lib/utils/api';
  import { buildAppUrl } from '$lib/utils/urlHelpers';
  import { localizeSpeciesName } from '$lib/utils/speciesDisplay';
  import { loggers } from '$lib/utils/logger';
  import {
    CALENDAR_EXPECTED_RATE,
    CALENDAR_WEEK_COUNT,
    calendarWeekPeriod,
    isBirdCalendarReport,
    type BirdCalendarReport,
    type CalendarSpecies,
  } from '$lib/utils/birdCalendar';

  const LOW_FREQUENCY_COLOR = '#152238';
  const HIGH_FREQUENCY_COLOR = '#4bafff';
  const REFRESH_INTERVAL_MS = 60_000;
  const heatmapColor = scaleSequential(
    interpolateRgb(LOW_FREQUENCY_COLOR, HIGH_FREQUENCY_COLOR)
  ).domain([0, 1]);
  const weeks = Array.from({ length: CALENDAR_WEEK_COUNT }, (_, week) => week);
  let report = $state<BirdCalendarReport | null>(null);
  let loading = $state(true);
  let failed = $state(false);
  let missing = $state(false);
  let selectedWeek = $state(0);
  let search = $state('');
  let filter = $state<'expected' | 'heard' | 'all'>('expected');
  let inspected = $state<CalendarSpecies | null>(null);
  let request: AbortController | undefined;
  let scroller = $state<HTMLDivElement>();

  $effect(() => {
    const week = selectedWeek;
    if (!report || !scroller) return;
    const header = scroller.querySelector(`[data-week-header="${week}"]`);
    if (header) {
      scroller.scrollLeft = Math.max(
        0,
        header.getBoundingClientRect().left -
          scroller.getBoundingClientRect().left +
          scroller.scrollLeft -
          scroller.clientWidth / 2
      );
    }
  });

  async function loadCalendar(background = false) {
    if (background && loading) return;
    request?.abort();
    const controller = new AbortController();
    request = controller;
    loading = !background || report === null;
    failed = false;
    missing = false;
    try {
      const value = await fetchWithCSRF<unknown>(buildAppUrl('/api/v2/species/calendar'), {
        signal: controller.signal,
        cache: 'no-store',
      });
      if (controller.signal.aborted) return;
      if (!isBirdCalendarReport(value)) throw new Error('Invalid calendar response');
      const firstLoad = report === null;
      report = value;
      if (firstLoad) selectedWeek = value.currentWeek;
      if (inspected) {
        inspected = value.species.find(bird => bird.scientific === inspected?.scientific) ?? null;
      }
    } catch (error) {
      if (controller.signal.aborted) return;
      if (error instanceof ApiError && error.status === 404) {
        report = null;
        missing = true;
        return;
      }
      failed = report === null;
      loggers.ui.error('Unable to load bird calendar', error);
    } finally {
      if (!controller.signal.aborted) loading = false;
    }
  }

  onMount(() => {
    void loadCalendar();
    const timer = setInterval(() => void loadCalendar(true), REFRESH_INTERVAL_MS);
    return () => {
      clearInterval(timer);
      request?.abort();
    };
  });

  const displayName = (bird: CalendarSpecies) => localizeSpeciesName(bird.scientific, bird.name);
  const rateAt = (bird: CalendarSpecies, week: number) => bird.rates.at(week) ?? null;
  const heardAt = (bird: CalendarSpecies, week: number) => bird.heard.at(week) ?? 0;
  const frequency = (value: number | null) =>
    value === null
      ? t('common.unknown')
      : new Intl.NumberFormat(getLocale(), { style: 'percent', maximumFractionDigits: 1 }).format(
          value
        );
  const rows = $derived.by(() => {
    const query = search.trim().toLocaleLowerCase(getLocale());
    return (report?.species ?? [])
      .filter(bird => {
        if (
          query &&
          !`${displayName(bird)} ${bird.scientific}`.toLocaleLowerCase(getLocale()).includes(query)
        )
          return false;
        if (filter === 'heard') return heardAt(bird, selectedWeek) > 0;
        if (filter === 'expected')
          return (
            (rateAt(bird, selectedWeek) ?? 0) >= CALENDAR_EXPECTED_RATE ||
            heardAt(bird, selectedWeek) > 0
          );
        return true;
      })
      .sort(
        (a, b) =>
          Number(heardAt(b, selectedWeek) > 0) - Number(heardAt(a, selectedWeek) > 0) ||
          (rateAt(b, selectedWeek) ?? 0) - (rateAt(a, selectedWeek) ?? 0) ||
          displayName(a).localeCompare(displayName(b), getLocale())
      );
  });
  const seasonalGroups = $derived([
    {
      id: 'arrivals',
      title: t('calendar.arrivals'),
      birds: report?.species.filter(bird => bird.arrivals.includes(selectedWeek)) ?? [],
    },
    {
      id: 'peak',
      title: t('calendar.peak'),
      birds: report?.species.filter(bird => bird.peaks.includes(selectedWeek)) ?? [],
    },
    {
      id: 'departures',
      title: t('calendar.departures'),
      birds: report?.species.filter(bird => bird.departures.includes(selectedWeek)) ?? [],
    },
  ]);

  function changeWeek(delta: number) {
    selectedWeek = (selectedWeek + delta + CALENDAR_WEEK_COUNT) % CALENDAR_WEEK_COUNT;
  }
</script>

<div class="col-span-12 min-w-0 space-y-6">
  <header class="flex items-start justify-between gap-4">
    <div>
      <h1 class="text-2xl font-bold">{t('calendar.title')}</h1>
      <p class="mt-1 opacity-70">{t('calendar.description')}</p>
    </div>
    <Button
      variant="ghost"
      onclick={() => void loadCalendar()}
      aria-label={t('common.refresh')}
      disabled={loading}><RefreshCw class="size-5" /></Button
    >
  </header>
  {#if loading}
    <p role="status">{t('common.loading')}</p>
  {:else if missing}
    <section class="rounded-xl bg-base-200 p-6">
      <h2 class="font-semibold">{t('calendar.missingTitle')}</h2>
      <p class="mt-2 opacity-70">{t('calendar.missingHint')}</p>
    </section>
  {:else if failed}
    <div role="alert">
      <p>{t('common.errors.unknownError')}</p>
      <Button className="mt-3" onclick={() => void loadCalendar()}>{t('common.retry')}</Button>
    </div>
  {:else if report}
    <section class="rounded-xl border border-base-300 bg-base-200 p-4 lg:p-6">
      <div class="flex flex-wrap items-center gap-3">
        <Button variant="ghost" aria-label={t('common.previous')} onclick={() => changeWeek(-1)}
          ><ChevronLeft class="size-5" /></Button
        >
        <select
          class="rounded-lg border border-base-300 bg-base-100 px-3 py-2 text-sm"
          aria-label={t('calendar.week', { week: selectedWeek + 1 })}
          bind:value={selectedWeek}
        >
          {#each weeks as week (week)}<option value={week}
              >{t('calendar.week', { week: week + 1 })} · {calendarWeekPeriod(
                week,
                getLocale()
              )}</option
            >{/each}
        </select>
        <Button variant="ghost" aria-label={t('common.next')} onclick={() => changeWeek(1)}
          ><ChevronRight class="size-5" /></Button
        >
        <span class="ml-auto text-sm opacity-70"
          >{report.region} · {report.firstHistoricalYear}–{report.lastHistoricalYear}</span
        >
      </div>
      <p class="mt-3 text-sm opacity-70">
        {t('calendar.evidence', {
          samples: report.samples.at(selectedWeek) ?? 0,
          years: report.years.at(selectedWeek) ?? 0,
        })} · {t('calendar.stationCoverage', { days: report.stationDays.at(selectedWeek) ?? 0 })}
      </p>
      <div class="mt-5 grid grid-cols-3 gap-4">
        {#each seasonalGroups as group (group.id)}
          <section class="rounded-lg bg-base-100 p-4">
            <h2 class="mb-3 font-semibold">{group.title}</h2>
            <ul class="max-h-48 space-y-2 overflow-auto text-sm">
              {#each group.birds as bird (bird.scientific)}<li>
                  {displayName(bird)}{#if heardAt(bird, selectedWeek)}
                    <span class="text-success"
                      >· {t('calendar.heard')}: {heardAt(bird, selectedWeek)}</span
                    >{/if}
                </li>{:else}<li class="opacity-60">{t('common.ui.noData')}</li>{/each}
            </ul>
          </section>
        {/each}
      </div>
      {#if inspected}
        <p class="mt-5 border-l-2 border-primary pl-3" aria-live="polite">
          <strong>{displayName(inspected)}</strong> · {calendarWeekPeriod(
            selectedWeek,
            getLocale()
          )} · {t('calendar.expected')}: {frequency(rateAt(inspected, selectedWeek))} · {t(
            'calendar.heard'
          )}: {heardAt(inspected, selectedWeek)}{#if !(report.stationDays.at(selectedWeek) ?? 0)}
            · {t('calendar.noCoverage')}{/if}
        </p>
      {/if}
    </section>
    <section class="rounded-xl border border-base-300 bg-base-200 p-4 lg:p-6">
      <div class="mb-4 flex flex-wrap items-center gap-3">
        <label
          class="flex min-w-48 flex-1 items-center gap-2 rounded-lg border border-base-300 bg-base-100 px-3 py-2 focus-within:ring-2 focus-within:ring-primary"
          ><Search class="size-4" /><input
            class="grow bg-transparent outline-none"
            type="search"
            aria-label={t('common.search')}
            placeholder={t('common.search')}
            bind:value={search}
          /></label
        >
        <select
          class="rounded-lg border border-base-300 bg-base-100 px-3 py-2 text-sm"
          aria-label={t('common.filter')}
          bind:value={filter}
          ><option value="expected">{t('calendar.expectedThisWeek')}</option><option value="heard"
            >{t('calendar.heardThisWeek')}</option
          ><option value="all">{t('calendar.allSpecies')}</option></select
        >
      </div>
      <p class="mb-3 text-sm opacity-70">
        {t('calendar.expected')} · <span aria-hidden="true">●</span>
        {t('calendar.heard')}
      </p>
      <div
        bind:this={scroller}
        class="max-h-[650px] overflow-auto rounded-lg border border-base-300"
      >
        <table
          class="calendar-table w-full border-separate border-spacing-1 text-xs"
          aria-label={t('calendar.title')}
        >
          <thead class="sticky top-0 z-20 bg-base-200"
            ><tr
              ><th class="species-name">{t('detections.headers.species')}</th
              >{#each weeks as week (week)}<th
                  data-week-header={week}
                  title={calendarWeekPeriod(week, getLocale())}>{week + 1}</th
                >{/each}</tr
            ></thead
          >
          <tbody>
            {#each rows as bird (bird.scientific)}
              <tr
                ><th scope="row" class="species-name text-left"
                  >{displayName(bird)}<span class="block text-[10px] font-normal italic opacity-60"
                    >{bird.scientific}</span
                  ></th
                >
                {#each weeks as week (week)}
                  {@const rate = rateAt(bird, week)}
                  <td
                    ><button
                      type="button"
                      class="calendar-cell"
                      class:chosen={week === selectedWeek}
                      class:unknown={rate === null}
                      style:background-color={rate === null ? undefined : heatmapColor(rate)}
                      tabindex={week === selectedWeek ? 0 : -1}
                      aria-label={`${displayName(bird)} · ${calendarWeekPeriod(week, getLocale())} · ${t('calendar.expected')}: ${frequency(rate)} · ${t('calendar.heard')}: ${heardAt(bird, week)}`}
                      title={`${frequency(rate)} · ${t('calendar.heard')}: ${heardAt(bird, week)}`}
                      onclick={() => {
                        selectedWeek = week;
                        inspected = bird;
                      }}
                      >{#if heardAt(bird, week)}<span aria-hidden="true">●</span>{/if}</button
                    ></td
                  >
                {/each}
              </tr>
            {:else}<tr
                ><td colspan={CALENDAR_WEEK_COUNT + 1} class="p-4">{t('common.ui.noData')}</td></tr
              >{/each}
          </tbody>
        </table>
      </div>
      <p class="mt-4 text-sm opacity-70">{t('calendar.caveat')}</p>
    </section>
    <footer class="space-y-2 text-xs opacity-60">
      <p>
        {t('calendar.snapshot', {
          date: new Intl.DateTimeFormat(getLocale(), {
            dateStyle: 'medium',
            timeStyle: 'short',
            timeZone: report.timezone,
          }).format(new Date(report.generated)),
        })} · {report.timezone} · {report.year}
      </p>
      <p>{report.citation}</p>
    </footer>
  {/if}
</div>

<style>
  .calendar-table {
    min-width: 1280px;
    table-layout: fixed;
  }

  .calendar-table td {
    padding: 2px 1px;
  }

  .species-name {
    position: sticky;
    left: 0;
    z-index: 10;
    width: 210px;
    padding: 6px;
    background: var(--color-base-200);
  }

  .calendar-cell {
    display: flex;
    width: 100%;
    min-height: 30px;
    align-items: center;
    justify-content: center;
    border-radius: 3px;
    color: white;
  }

  .calendar-cell.chosen {
    outline: 2px solid var(--color-primary);
  }

  .calendar-cell:focus-visible {
    outline: 3px solid var(--color-primary);
    outline-offset: 2px;
  }

  .calendar-cell.unknown {
    background: repeating-linear-gradient(
      135deg,
      var(--color-base-300),
      var(--color-base-300) 3px,
      var(--color-base-100) 3px,
      var(--color-base-100) 6px
    );
  }
</style>

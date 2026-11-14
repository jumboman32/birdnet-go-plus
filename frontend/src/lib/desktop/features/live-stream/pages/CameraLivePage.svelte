<script lang="ts">
  import { onMount } from 'svelte';
  import { Camera, Pause, Play, RefreshCw, Settings, VideoOff } from '@lucide/svelte';
  import Card from '$lib/desktop/components/ui/Card.svelte';
  import CameraObservationResults, {
    type VisualObservation,
  } from '../components/CameraObservationResults.svelte';
  import ErrorAlert from '$lib/desktop/components/ui/ErrorAlert.svelte';
  import SelectDropdown from '$lib/desktop/components/forms/SelectDropdown.svelte';
  import SourceCameraVideo from '../components/SourceCameraVideo.svelte';
  import { api } from '$lib/utils/api';
  import { buildAppUrl } from '$lib/utils/urlHelpers';
  import { t } from '$lib/i18n';

  interface CameraInfo {
    name: string;
    online: boolean;
    fps: number;
  }
  interface CameraResponse {
    enabled: boolean;
    cameras: CameraInfo[];
  }
  const OBSERVATIONS_API = '/api/v2/visual-detections?limit=20';
  let observations = $state<VisualObservation[]>([]);
  let observationsLoading = $state(true);
  let observationsError = $state('');

  const STATUS_INTERVAL = 10_000;
  const STREAM_INTERVAL = 25_000;
  const SOURCE_STREAM_INTERVAL = 9 * 60_000;
  const READY_INTERVAL = 500;
  const CAMERAS_API = '/api/v2/integrations/frigate/cameras';
  let data = $state<CameraResponse | null>(null);
  let selected = $state('');
  let paused = $state(false);
  let visible = $state(true);
  let error = $state('');
  let streamError = $state(false);
  let ready = $state(false);
  let streamSource = $state('');
  let streamAttempt = $state(0);
  let quality = $state('preview');
  let sourceFallback = $state(false);
  let sourceReady = $state(false);
  let refreshing = $state(false);
  let refresh: () => void = () => {};
  let image: HTMLImageElement | undefined = $state();
  let camera = $derived(data?.cameras.find(item => item.name === selected));
  let active = $derived(Boolean(data?.enabled && camera?.online && !paused && visible && !error));
  let options = $derived(data?.cameras.map(item => ({ value: item.name, label: item.name })) ?? []);
  const settingsURL = buildAppUrl('/ui/settings/integrations?tab=frigate');
  let sourceMode = $derived(quality === 'source' && !sourceFallback);
  let qualityOptions = $derived([
    { value: 'preview', label: t('cameraLive.previewQuality') },
    { value: 'source', label: t('cameraLive.sourceQuality') },
  ]);
  let live = $derived(active && (sourceMode ? sourceReady : ready) && !streamError);
  let status = $derived(
    error || streamError
      ? t('cameraLive.unavailable')
      : paused || !visible
        ? t('cameraLive.paused')
        : !camera?.online
          ? t('cameraLive.offline')
          : live
            ? t('cameraLive.live')
            : t('cameraLive.connecting')
  );
  function retry() {
    if (refreshing) return;
    sourceFallback = false;
    sourceReady = false;
    streamAttempt++;
    refresh();
  }

  onMount(() => {
    let disposed = false;
    let generation = 0;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let request: AbortController | undefined;
    async function load() {
      clearTimeout(timer);
      const current = ++generation;
      request?.abort();
      request = new AbortController();
      refreshing = true;
      try {
        const [cameras, visuals] = await Promise.allSettled([
          api.get<CameraResponse>(CAMERAS_API, { signal: request.signal }),
          api.get<{ data: VisualObservation[] }>(OBSERVATIONS_API, { signal: request.signal }),
        ]);
        if (disposed || current !== generation) return;
        if (cameras.status === 'fulfilled') {
          data = cameras.value;
          error = '';
          if (!cameras.value.cameras.some(item => item.name === selected))
            selected = cameras.value.cameras[0]?.name ?? '';
        } else {
          error = t('cameraLive.connectionError');
        }
        observationsLoading = false;
        if (visuals.status === 'fulfilled') {
          observations = visuals.value.data;
          observationsError = '';
        } else {
          observationsError = t('cameraLive.observationsError');
        }
      } finally {
        if (!disposed && current === generation) refreshing = false;
        if (!disposed && current === generation && visible)
          timer = setTimeout(load, STATUS_INTERVAL);
      }
    }
    refresh = () => {
      if (!disposed && visible) void load();
    };
    function visibilityChanged() {
      visible = !document.hidden;
      generation++;
      request?.abort();
      clearTimeout(timer);
      if (visible) void load();
    }
    visible = !document.hidden;
    if (visible) void load();
    document.addEventListener('visibilitychange', visibilityChanged);
    return () => {
      disposed = true;
      refresh = () => {};
      clearTimeout(timer);
      request?.abort();
      document.removeEventListener('visibilitychange', visibilityChanged);
    };
  });

  $effect(() => {
    const name = selected;
    const attempt = streamAttempt;
    sourceReady = false;
    ready = false;
    streamError = false;
    if (!active || !name) {
      streamSource = '';
      return;
    }
    if (sourceMode) {
      streamSource = '';
      const renew = setInterval(() => {
        streamAttempt++;
      }, SOURCE_STREAM_INTERVAL);
      return () => clearInterval(renew);
    }
    function connect() {
      streamError = false;
      streamSource =
        buildAppUrl(`${CAMERAS_API}/${encodeURIComponent(name)}/live`) +
        `?session=${Date.now()}-${attempt}`;
    }
    connect();
    const renew = setInterval(connect, STREAM_INTERVAL);
    // Some browsers report multipart-image load only when the connection ends.
    const check = setInterval(() => {
      if (image?.naturalWidth && !streamError) ready = true;
    }, READY_INTERVAL);
    return () => {
      clearInterval(renew);
      clearInterval(check);
    };
  });
</script>

<svelte:head><title>{t('cameraLive.title')}</title></svelte:head>

<div class="col-span-12 min-w-0 w-full space-y-5">
  <div class="flex items-start justify-between gap-4">
    <div>
      <h1 class="text-2xl font-semibold flex items-center gap-2">
        <Camera size={26} />{t('cameraLive.title')}
      </h1>
      <p class="mt-1 text-sm opacity-70">{t('cameraLive.description')}</p>
    </div>
    <div class="flex shrink-0 items-center gap-2">
      <button
        type="button"
        class="inline-flex items-center gap-2 rounded-lg border border-[var(--border-200)] px-3 py-2 text-sm hover:bg-[var(--color-base-200)] focus-visible:outline-2 focus-visible:outline-offset-2"
        onclick={retry}
        aria-disabled={refreshing}
      >
        <RefreshCw size={16} class={refreshing ? 'animate-spin' : ''} />
        {refreshing ? t('common.loading') : t('common.refresh')}
      </button>
      <a
        class="inline-flex items-center gap-2 rounded-lg border border-[var(--border-200)] px-3 py-2 text-sm hover:bg-[var(--color-base-200)] focus-visible:outline-2 focus-visible:outline-offset-2"
        href={settingsURL}><Settings size={16} />{t('cameraLive.settings')}</a
      >
    </div>
  </div>
  {#if error}<ErrorAlert message={error} />{/if}
  {#if !data && !error}
    <p role="status">{t('common.loading')}</p>
  {:else if data && !data.enabled}
    <Card
      ><p>{t('cameraLive.disabled')}</p>
      <a class="underline" href={settingsURL}>{t('cameraLive.settings')}</a></Card
    >
  {:else if data && data.cameras.length === 0}
    <Card
      ><p>{t('cameraLive.empty')}</p>
      <a class="underline" href={settingsURL}>{t('cameraLive.settings')}</a></Card
    >
  {:else if data}
    <Card padding={false}>
      <div
        class="flex flex-wrap items-end justify-between gap-4 border-b border-[var(--border-200)] p-4"
      >
        <div class="w-64 max-w-full">
          <SelectDropdown
            label={t('cameraLive.camera')}
            value={selected}
            {options}
            onChange={value => {
              if (typeof value === 'string') {
                selected = value;
                sourceFallback = false;
              }
            }}
          />
        </div>
        <div class="w-64 max-w-full">
          <SelectDropdown
            label={t('cameraLive.qualityLabel')}
            value={quality}
            options={qualityOptions}
            onChange={value => {
              if (value === 'preview' || value === 'source') {
                quality = value;
                sourceFallback = false;
                sourceReady = false;
              }
            }}
          />
        </div>
        <div class="flex items-center gap-4">
          <span class="text-sm tabular-nums opacity-70"
            >{t('cameraLive.detectionFps')}: {Number.isFinite(camera?.fps)
              ? camera?.fps.toFixed(1)
              : '—'}</span
          >
          <button
            type="button"
            class="inline-flex items-center gap-2 rounded-lg border border-[var(--border-200)] px-4 py-2 text-sm hover:bg-[var(--color-base-200)] focus-visible:outline-2 focus-visible:outline-offset-2"
            onclick={() => {
              paused = !paused;
            }}
          >
            {#if paused}<Play size={16} />{t('cameraLive.resume')}{:else}<Pause size={16} />{t(
                'cameraLive.pause'
              )}{/if}
          </button>
        </div>
      </div>
      <div class="border-b border-[var(--border-200)] px-4 py-2 text-sm opacity-80">
        {#if sourceFallback}<p role="status">{t('cameraLive.sourceFallback')}</p>
        {:else}<p>{sourceMode ? t('cameraLive.sourceHelp') : t('cameraLive.previewHelp')}</p>{/if}
      </div>
      <div class="relative aspect-video max-h-[70vh] overflow-hidden bg-black">
        {#if active && sourceMode}
          {#key `${selected}:${streamAttempt}`}
            <SourceCameraVideo
              camera={selected}
              source={buildAppUrl(`${CAMERAS_API}/${encodeURIComponent(selected)}/source`)}
              onReady={() => {
                sourceReady = true;
              }}
              onError={() => {
                sourceFallback = true;
                sourceReady = false;
              }}
            />
          {/key}
        {:else if active && streamSource}
          {#key selected}
            <img
              bind:this={image}
              src={streamSource}
              alt={selected}
              class="h-full w-full object-contain"
              onload={() => {
                ready = true;
              }}
              onerror={() => {
                streamError = true;
                ready = false;
              }}
            />
          {/key}
        {/if}
        {#if !camera?.online || paused || !visible || !(sourceMode ? sourceReady : ready) || streamError}
          <div
            class="absolute inset-0 flex items-center justify-center p-6 text-center text-white bg-black/50"
          >
            <div role="status">
              <VideoOff size={36} class="mx-auto mb-4 opacity-60" />
              {#if error}<p class="text-lg">{t('cameraLive.unavailable')}</p>
              {:else if !camera?.online}<p class="text-lg">{t('cameraLive.offline')}</p>
                <p class="mt-2 text-sm">{t('cameraLive.offlineHelp')}</p>
              {:else if paused || !visible}<p>{t('cameraLive.paused')}</p>
              {:else if streamError}<p>{t('cameraLive.unavailable')}</p>
              {:else}<p>{t('cameraLive.connecting')}</p>{/if}
              {#if streamError || error}
                <button
                  type="button"
                  class="mt-4 inline-flex items-center gap-2 rounded-lg border border-white/40 px-4 py-2 hover:bg-white/10 focus-visible:outline-2 focus-visible:outline-offset-2"
                  onclick={retry}
                  aria-disabled={refreshing}
                >
                  <RefreshCw size={16} />{refreshing ? t('common.loading') : t('common.retry')}
                </button>
              {/if}
            </div>
          </div>
        {/if}
      </div>
      <div class="flex items-center justify-between gap-4 px-4 py-3 text-sm">
        <span role="status" class="inline-flex items-center gap-2"
          ><span
            aria-hidden="true"
            class="h-2 w-2 rounded-full {live ? 'bg-emerald-500' : 'bg-amber-500'}"
          ></span>{status}</span
        >
        <span class="inline-flex items-center gap-2 opacity-70"
          ><Camera size={15} />{sourceMode
            ? t('cameraLive.sourceQuality')
            : t('cameraLive.previewQuality')} · {t('cameraLive.videoOnly')}</span
        >
      </div>
    </Card>
  {/if}
  <CameraObservationResults
    data={observations}
    loading={observationsLoading}
    error={observationsError}
  />
</div>

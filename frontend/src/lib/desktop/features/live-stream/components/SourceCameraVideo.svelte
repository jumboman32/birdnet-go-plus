<script lang="ts">
  import { connectSourceVideo } from '../services/sourceVideo';
  interface Props {
    camera: string;
    source: string;
    onReady: () => void;
    onError: () => void;
  }
  let { camera, source, onReady, onError }: Props = $props();
  let video: HTMLVideoElement | undefined = $state();
  $effect(() => {
    if (video) return connectSourceVideo(video, source, onError);
  });
</script>

<video
  bind:this={video}
  aria-label={camera}
  class="h-full w-full object-contain"
  autoplay
  muted
  playsinline
  onplaying={onReady}
></video>

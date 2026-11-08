<!--
  MoonBadge - Compact moon phase emoji badge for detection cards.
  Displayed alongside the existing WeatherBadge.
  @component
-->
<script lang="ts">
  import { t } from '$lib/i18n';
  import { getMoonPhaseEmoji, getMoonPhaseI18nKey } from '$lib/utils/weather';

  interface Props {
    moonPhaseName: string;
    className?: string;
  }

  let { moonPhaseName, className = '' }: Props = $props();

  let emoji = $derived(getMoonPhaseEmoji(moonPhaseName));
  let translatedName = $derived(t(`weather.moon.${getMoonPhaseI18nKey(moonPhaseName)}`));
</script>

<div
  class="flex self-stretch items-center justify-center rounded-full bg-black/50 px-2 py-1 backdrop-blur-sm {className}"
  title={translatedName}
  role="img"
  aria-label={translatedName}
>
  <span class="text-base leading-none">{emoji}</span>
</div>

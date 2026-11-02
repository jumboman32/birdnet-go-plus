import { afterEach, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, screen } from '@testing-library/svelte';
import { renderTyped } from '../../../test/render-helpers';
import LocalRarityPreview from './LocalRarityPreview.svelte';

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it('labels sample data and switches the actual rarity card without bootstrapping the app', async () => {
  const fetch = vi.fn().mockImplementation(async (url: string) => {
    const rare =
      new URL(url, 'http://preview.internal').searchParams.get('scientific_name') === 'rare';
    return new Response(
      JSON.stringify({
        status: 'available',
        reason: 'SYNTHETIC PREVIEW DATA',
        reasonCode: 'available',
        score: rare ? 98 : 75,
        scoreLow: rare ? 94 : 68,
        scoreHigh: rare ? 100 : 82,
        samples: 120,
        reports: rare ? 2 : 30,
        years: 3,
        regionName: 'Atlantic County',
        radiusKm: 25,
        source: 'SYNTHETIC PREVIEW DATA',
        retrievedAt: '2026-11-01T00:00:00Z',
        recentStatus: 'available',
        recentReported: !rare,
        birdcastUrl: '',
      })
    );
  });
  vi.stubGlobal('fetch', fetch);
  renderTyped(LocalRarityPreview);
  expect(screen.getByText('species.localRarity.preview.notice')).toBeInTheDocument();
  await screen.findByText('75');
  await fireEvent.change(screen.getByLabelText('species.localRarity.preview.scenario'), {
    target: { value: 'rare' },
  });
  await screen.findByText('98');
  for (const [url] of fetch.mock.calls) expect(url).toContain('/api/v2/species/local-rarity?');
});

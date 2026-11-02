import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, screen, waitFor } from '@testing-library/svelte';
import { createComponentTestFactory, renderTyped } from '../../../test/render-helpers';
import LocalRarityCard from './LocalRarityCard.svelte';
import { tick } from 'svelte';
import { t } from '$lib/i18n';

const scientificName = 'Cardinalis cardinalis';
const baseline = {
  status: 'available',
  score: 75,
  scoreLow: 68,
  scoreHigh: 82,
  reason: 'Seasonal reporting rarity.',
  reasonCode: 'available',
  samples: 120,
  reports: 30,
  years: 3,
  regionName: 'Atlantic County',
  radiusKm: 25,
  source: 'Synthetic eBird fixture',
  retrievedAt: '2026-01-01T00:00:00Z',
  recentStatus: 'available',
  recentReported: true,
  birdcastUrl: 'https://dashboard.birdcast.org/region/US-NJ-001?night=2026-04-30',
};
const props = { scientificName, date: '2026-05-01' };

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

describe('LocalRarityCard', () => {
  it('separates score, sampling uncertainty, recent evidence and BirdCast context', async () => {
    const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify(baseline)));
    vi.stubGlobal('fetch', fetch);
    renderTyped(LocalRarityCard, { props });
    await screen.findByText('75');
    expect(screen.getByText('species.localRarity.interpretation')).toBeInTheDocument();
    expect(t).toHaveBeenCalledWith('species.localRarity.counts', {
      reports: 30,
      samples: 120,
      years: 3,
    });
    expect(screen.getByRole('link', { name: 'species.localRarity.birdcastLink' })).toHaveAttribute(
      'href',
      baseline.birdcastUrl
    );
    expect(fetch.mock.calls[0][0]).toContain('scientific_name=Cardinalis+cardinalis');
    expect(fetch.mock.calls[0][0]).toContain('date=2026-05-01');
  });

  it('renders missing data without substituting a zero score or claiming absence', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            ...baseline,
            status: 'insufficient_data',
            score: null,
            scoreLow: null,
            scoreHigh: null,
            reason: 'Historical data has not been supplied.',
            reasonCode: 'missing_data',
            recentReported: false,
          })
        )
      )
    );
    renderTyped(LocalRarityCard, { props });
    await screen.findByText('species.localRarity.insufficient');
    expect(screen.getByText(/species.localRarity.recentMissing/)).toBeInTheDocument();
    expect(screen.queryByText('/ 100')).not.toBeInTheDocument();
  });

  it('provides a retry after a failed request and rejects unsafe external links', async () => {
    const fetch = vi
      .fn()
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ ...baseline, birdcastUrl: 'https://example.com/' }))
      );
    vi.stubGlobal('fetch', fetch);
    renderTyped(LocalRarityCard, { props });
    await fireEvent.click(await screen.findByRole('button', { name: 'species.localRarity.retry' }));
    await screen.findByText('75');
    expect(screen.queryByRole('link')).not.toBeInTheDocument();
  });

  it('aborts a previous detection request and ignores its late response', async () => {
    let finish: ((response: Response) => void) | undefined;
    const old = new Promise<Response>(resolve => {
      finish = resolve;
    });
    const fetch = vi
      .fn()
      .mockReturnValueOnce(old)
      .mockResolvedValueOnce(new Response(JSON.stringify({ ...baseline, score: 30 })));
    vi.stubGlobal('fetch', fetch);
    const view = createComponentTestFactory(LocalRarityCard).render(props);
    await waitFor(() => expect(fetch).toHaveBeenCalledTimes(1));
    await view.rerender({ scientificName: 'Cyanocitta cristata', date: props.date });
    await screen.findByText('30');
    expect(fetch.mock.calls[0][1].signal.aborted).toBe(true);
    const stale = new Response(JSON.stringify(baseline));
    const json = vi.spyOn(stale, 'json');
    finish?.(stale);
    await waitFor(() => expect(json).toHaveBeenCalledOnce());
    await json.mock.results[0].value;
    await tick();
    expect(screen.queryByText('75')).not.toBeInTheDocument();
  });
});

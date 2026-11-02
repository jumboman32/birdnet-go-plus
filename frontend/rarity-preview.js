// Development-only sample responses. This plugin is never enabled for builds.
export const RARITY_PREVIEW_ENTRY = '/rarity-preview.html';
const RARITY_ENDPOINT = '/api/v2/species/local-rarity';
const ASSET_PREFIX = '/ui/assets/';
const SAMPLE_DATE = '2026-05-01';

/** @returns {import('vite').Plugin} */
export function rarityPreviewPlugin() {
  return {
    name: 'isolated-rarity-preview',
    apply: 'serve',
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        const url = new URL(req.url ?? '/', 'http://preview.internal');
        if (
          url.pathname === '/' ||
          (url.pathname.startsWith('/ui/') && !url.pathname.startsWith(ASSET_PREFIX))
        ) {
          res.writeHead(302, { Location: RARITY_PREVIEW_ENTRY });
          res.end();
          return;
        }
        if (!url.pathname.startsWith('/api/')) return next();
        res.setHeader('Content-Type', 'application/json');
        res.setHeader('Cache-Control', 'no-store');
        // No API request can reach a real backend from this preview server.
        if (req.method !== 'GET' || url.pathname !== RARITY_ENDPOINT) {
          res.statusCode = 404;
          res.end(JSON.stringify({ error: 'API unavailable in isolated preview' }));
          return;
        }
        const scenario = url.searchParams.get('scientific_name');
        if (scenario === 'unavailable') {
          res.statusCode = 503;
          res.end(JSON.stringify({ error: 'Simulated unavailable backend' }));
          return;
        }
        const missing = scenario === 'missing';
        const rare = scenario === 'rare';
        res.end(
          JSON.stringify({
            status: missing ? 'insufficient_data' : 'available',
            reason: 'SYNTHETIC PREVIEW DATA',
            reasonCode: missing ? 'missing_data' : 'available',
            score: missing ? null : rare ? 98 : 75,
            scoreLow: missing ? null : rare ? 94 : 68,
            scoreHigh: missing ? null : rare ? 100 : 82,
            samples: missing ? 0 : 120,
            reports: missing ? 0 : rare ? 2 : 30,
            years: missing ? 0 : 3,
            regionName: 'Atlantic County, New Jersey',
            radiusKm: 25,
            source: 'SYNTHETIC PREVIEW DATA',
            retrievedAt: '2026-11-01T00:00:00Z',
            method: 'preview-fixture',
            date: SAMPLE_DATE,
            recentStatus: scenario === 'historical' ? 'historical_unavailable' : 'available',
            recentReported: !missing && !rare && scenario !== 'historical',
            birdcastUrl: 'https://dashboard.birdcast.org/region/US-NJ-001?night=2026-04-30',
          })
        );
      });
    },
  };
}

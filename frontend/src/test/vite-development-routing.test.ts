import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { request } from 'node:http';
import { resolve } from 'node:path';
import { createServer, resolveConfig, type ViteDevServer } from 'vite';

describe('Vite development routes', () => {
  let server: ViteDevServer | undefined;
  let origin: string;

  beforeAll(async () => {
    server = await createServer({
      configFile: resolve('vite.config.js'),
      logLevel: 'silent',
      server: { host: '127.0.0.1', port: 0, open: false, watch: null },
    });
    await server.listen();
    const address = server.httpServer?.address();
    if (!address || typeof address === 'string')
      throw new Error('Dev server did not bind a TCP port');
    origin = `http://127.0.0.1:${address.port}`;
  });

  afterAll(async () => {
    await server?.close();
  });

  function get(
    path: string,
    targetOrigin = origin,
    method = 'GET'
  ): Promise<{ status: number; location?: string; body: string }> {
    return new Promise((resolveResponse, reject) => {
      const req = request(targetOrigin + path, { method }, response => {
        let body = '';
        response.setEncoding('utf8');
        response.on('data', chunk => {
          body += chunk;
        });
        response.on('end', () =>
          resolveResponse({
            status: response.statusCode ?? 0,
            location: response.headers.location,
            body,
          })
        );
        response.on('error', reject);
      });
      req.on('error', reject);
      req.end();
    });
  }

  it('serves normal page routes and refreshes through the SPA entry', async () => {
    for (const path of ['/ui/dashboard', '/ui/detections', '/ui/detections/1', '/ui/settings']) {
      const response = await get(path);
      expect(response.status).toBe(200);
      expect(response.body).toContain('id="app"');
      expect(response.body).toContain('/src/main.js');
    }
  });

  it('redirects the previously suggested asset URL to the dashboard', async () => {
    for (const path of ['/ui/assets', '/ui/assets/']) {
      const response = await get(path + '?test=1');
      expect(response.status).toBe(302);
      expect(response.location).toBe('/ui/dashboard?test=1');
    }
  });

  it('serves translation JSON at the runtime production-style asset URL', async () => {
    const response = await get('/ui/assets/messages/en.json?v=test');
    expect(response.status).toBe(200);
    expect(JSON.parse(response.body).species.localRarity.title).toBe('Local Rarity Score');
  });

  it('preserves the production build asset base', async () => {
    const config = await resolveConfig(
      { configFile: resolve('vite.config.js'), logLevel: 'silent' },
      'build'
    );
    expect(config.base).toBe('/ui/assets/');
  });

  it('rejects the sample HTML entry when the real backend proxy is enabled', async () => {
    expect((await get('/rarity-preview.html')).status).toBe(404);
  });

  it('preserves preview paths without opening a development-only route', async () => {
    const config = await resolveConfig(
      { configFile: resolve('vite.config.js'), logLevel: 'silent' },
      'serve',
      'production',
      'production',
      true
    );
    expect(config.base).toBe('/ui/assets/');
    expect(config.preview.open).toBe(false);
  });

  it('runs an isolated sample preview without a production API proxy', async () => {
    const preview = await createServer({
      configFile: resolve('vite.config.js'),
      mode: 'rarity',
      logLevel: 'silent',
      server: { host: '127.0.0.1', port: 0, open: false, watch: null },
    });
    try {
      await preview.listen();
      const address = preview.httpServer?.address();
      if (!address || typeof address === 'string') throw new Error('Preview did not bind a port');
      const previewOrigin = `http://127.0.0.1:${address.port}`;
      expect(preview.config.server.proxy).toBeUndefined();
      const root = await get('/', previewOrigin);
      expect(root.location).toBe('/rarity-preview.html');
      const html = await get('/rarity-preview.html', previewOrigin);
      expect(html.status).toBe(200);
      expect(html.body).toContain('/src/rarity-preview.ts');
      expect(html.body).not.toContain('/src/main.js');
      const endpoint = '/api/v2/species/local-rarity?scientific_name=';
      const available = await get(endpoint + 'available', previewOrigin);
      expect(JSON.parse(available.body).score).toBe(75);
      expect(JSON.parse(available.body).source).toBe('SYNTHETIC PREVIEW DATA');
      const rare = await get(endpoint + 'rare', previewOrigin);
      expect(JSON.parse(rare.body).score).toBe(98);
      const missing = await get(endpoint + 'missing', previewOrigin);
      expect(JSON.parse(missing.body).score).toBeNull();
      const historical = await get(endpoint + 'historical', previewOrigin);
      expect(JSON.parse(historical.body).recentStatus).toBe('historical_unavailable');
      expect((await get(endpoint + 'unavailable', previewOrigin)).status).toBe(503);
      expect((await get('/api/v2/app/config', previewOrigin)).status).toBe(404);
      expect((await get('/api/v2/settings', previewOrigin, 'PUT')).status).toBe(404);
    } finally {
      await preview.close();
    }
  });

  it('never enables sample endpoints for a production build', async () => {
    const config = await resolveConfig(
      { configFile: resolve('vite.config.js'), mode: 'rarity', logLevel: 'silent' },
      'build'
    );
    expect(config.plugins.some(plugin => plugin.name === 'isolated-rarity-preview')).toBe(false);
  });
});

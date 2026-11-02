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

  function get(path: string): Promise<{ status: number; location?: string; body: string }> {
    return new Promise((resolveResponse, reject) => {
      const req = request(origin + path, response => {
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
});

import test from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, copyFile, writeFile, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";
import { execFile } from "node:child_process";

const run = promisify(execFile);

test(
  "launcher isolates config, database, models and inherited backend settings",
  { skip: process.platform === "win32" },
  async () => {
    const root = await mkdtemp(join(tmpdir(), "birdnet-launcher-"));
    try {
      await copyFile(
        new URL("./start-development.mjs", import.meta.url),
        join(root, "start-development.mjs"),
      );
      await writeFile(
        join(root, "birdnet-go-dev"),
        `#!/usr/bin/env node
const fs = require('node:fs');
fs.writeFileSync('invocation.json', JSON.stringify({args:process.argv.slice(2),cwd:process.cwd(),env:Object.fromEntries(Object.entries(process.env).filter(([k])=>k.startsWith('BIRDNET_')))}));
`,
        { mode: 0o700 },
      );
      await run(process.execPath, [join(root, "start-development.mjs")], {
        env: {
          ...process.env,
          BIRDNET_WEBSERVER_PORT: "8080",
          BIRDNET_MODELPATH: "/production/model",
          BIRDNET_LOCAL_RARITY_DATA: join(root, "historical.json"),
        },
      });
      const data = join(root, "development-data");
      const config = JSON.parse(
        await readFile(join(data, "config.yaml"), "utf8"),
      );
      const invocation = JSON.parse(
        await readFile(join(data, "invocation.json"), "utf8"),
      );
      assert.deepEqual(invocation.args, [
        "--config",
        join(data, "config.yaml"),
        "serve",
      ]);
      assert.equal(invocation.cwd, data);
      assert.deepEqual(invocation.env, {
        BIRDNET_WEBSERVER_PORT: "8081",
        BIRDNET_RARITY_REGION: "US-NJ-001",
        BIRDNET_LOCAL_RARITY_DATA: join(root, "historical.json"),
      });
      assert.equal(config.output.sqlite.path, join(data, "birdnet-dev.db"));
      assert.equal(config.models.directory, join(data, "models"));
      assert.deepEqual(config.models.enabled, []);
      assert.equal(config.models.autoenablemigrated, true);
      assert.equal(config.configversion, 2);
      assert.equal(config.realtime.weather.provider, "none");
      // An existing development configuration (including a locally entered key)
      // must survive a later launch byte for byte.
      await writeFile(
        join(data, "config.yaml"),
        "development-setting: retained\n",
      );
      await run(process.execPath, [join(root, "start-development.mjs")]);
      assert.equal(
        await readFile(join(data, "config.yaml"), "utf8"),
        "development-setting: retained\n",
      );
    } finally {
      await rm(root, { recursive: true, force: true });
    }
  },
);

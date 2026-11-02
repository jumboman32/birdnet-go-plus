// Packaged beside the development executable. Never discovers production config.
import { spawn } from "node:child_process";
import { mkdir, writeFile, access } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { dirname, join, resolve } from "node:path";
import { createServer } from "node:net";

const root = dirname(fileURLToPath(import.meta.url));
const data = join(root, "development-data");
const config = join(data, "config.yaml");
const port = 8081;
const executable = join(
  root,
  process.platform === "win32" ? "birdnet-go-dev.exe" : "birdnet-go-dev",
);

async function main() {
  await access(executable);
  // Fail clearly rather than accidentally opening another running backend.
  await new Promise((accept, reject) => {
    const probe = createServer();
    probe.once("error", reject);
    probe.listen(port, () => probe.close(accept));
  });
  await mkdir(data, { recursive: true });
  const initialConfig = {
    configversion: 2,
    birdnet: { latitude: 39.37, longitude: -74.57 },
    models: {
      enabled: [],
      directory: join(data, "models"),
      autoenablemigrated: true,
    },
    realtime: {
      audio: { sources: [] },
      birdweather: { enabled: false },
      mqtt: { enabled: false },
      ebird: { enabled: false },
      weather: { provider: "none" },
      telemetry: { enabled: false },
    },
    webserver: { enabled: true, port: String(port) },
    output: {
      sqlite: { enabled: true, path: join(data, "birdnet-dev.db") },
      mysql: { enabled: false },
    },
    sentry: { enabled: false },
  };
  try {
    // JSON is valid YAML; the backend can subsequently save normal YAML here.
    await writeFile(config, JSON.stringify(initialConfig, null, 2), {
      flag: "wx",
      mode: 0o600,
    });
  } catch (error) {
    if (error.code !== "EEXIST") throw error;
  }
  const environment = Object.fromEntries(
    Object.entries(process.env).filter(
      ([key]) => !key.toUpperCase().startsWith("BIRDNET_"),
    ),
  );
  environment.BIRDNET_WEBSERVER_PORT = String(port);
  environment.BIRDNET_RARITY_REGION = "US-NJ-001";
  if (process.env.BIRDNET_LOCAL_RARITY_DATA) {
    environment.BIRDNET_LOCAL_RARITY_DATA = resolve(
      process.env.BIRDNET_LOCAL_RARITY_DATA,
    );
  }
  console.log(`Development backend: http://localhost:${port}/ui/dashboard`);
  console.log(`Separate development files: ${data}`);
  console.log(
    "No microphone or acoustic model is enabled initially. Configure them in this development UI if needed.",
  );
  console.log(
    "Enter a replacement eBird key only in this development backend’s integration settings.",
  );
  console.log(
    "Numeric rarity requires an authorized historical eBird import; the API key alone is insufficient.",
  );
  const backend = spawn(executable, ["--config", config, "serve"], {
    cwd: data,
    env: environment,
    stdio: "inherit",
  });
  backend.once("error", (error) => {
    console.error(error.message);
    process.exitCode = 1;
  });
  backend.once("exit", (code) => {
    process.exitCode = code ?? 1;
  });
  // Windows delivers console Ctrl+C to both processes. Forwarding with kill()
  // there would forcibly terminate Go before it can flush its database.
  process.once("SIGINT", () => {
    if (process.platform !== "win32") backend.kill("SIGINT");
  });
  process.once("SIGTERM", () => backend.kill("SIGTERM"));
}

main().catch((error) => {
  console.error(`Cannot start development backend: ${error.message}`);
  process.exitCode = 1;
});

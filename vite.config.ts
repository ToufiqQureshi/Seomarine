import { tanstackRouter } from "@tanstack/router-plugin/vite";
import { defineConfig } from "vitest/config";
import { loadEnv } from "vite";
import tsConfigPaths from "vite-tsconfig-paths";
import viteReact from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig(({ mode }) => {
  // Vitest sets mode to "test": only the tsconfig path alias is needed.
  if (mode === "test") {
    return {
      plugins: [tsConfigPaths()],
      test: {
        environment: "node",
        include: ["src/**/*.test.ts", "scripts/**/*.test.ts"],
        restoreMocks: true,
        clearMocks: true,
      },
    };
  }

  const env = loadEnv(mode, process.cwd(), "");
  const port = process.env.PORT
    ? Number(process.env.PORT)
    : env.PORT
      ? Number(env.PORT)
      : 3001;
  // In development the Go server owns every non-asset path.
  const goServer = env.GO_SERVER_URL || "http://localhost:8080";
  const proxied = Object.fromEntries(
    ["/api", "/s", "/r", "/mcp", "/webhooks", "/t.js", "/.well-known"].map(
      (path) => [path, { target: goServer, changeOrigin: false }],
    ),
  );
  const emitSourcemaps = env.POSTHOG_SOURCEMAPS === "true";

  return {
    // Static files (favicons, manifest) live beside the app code instead of at
    // the repo root.
    publicDir: "src/public",
    envPrefix: [
      "VITE_",
      "AUTH_MODE",
      "BYPASS_EMAIL_VERIFICATION",
      "POSTHOG_PUBLIC_KEY",
      "POSTHOG_HOST",
      "TURNSTILE_SITE_KEY",
    ],
    server: { port, proxy: proxied },
    preview: { port, proxy: proxied },
    build: {
      sourcemap: emitSourcemaps,
      outDir: emitSourcemaps ? "dist-sourcemaps" : "dist",
    },
    plugins: [
      tanstackRouter({
        target: "react",
        autoCodeSplitting: true,
        routesDirectory: "src/routes",
        generatedRouteTree: "src/routeTree.gen.ts",
      }),
      tsConfigPaths(),
      viteReact(),
      tailwindcss(),
    ],
  };
});

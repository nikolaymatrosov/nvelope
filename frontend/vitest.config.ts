import { defineConfig } from "vitest/config"
import viteReact from "@vitejs/plugin-react"
import viteTsConfigPaths from "vite-tsconfig-paths"
import { storybookTest } from "@storybook/addon-vitest/vitest-plugin"
import { playwright } from "@vitest/browser-playwright"

export default defineConfig({
  test: {
    projects: [
      // Project 1 — existing jsdom unit tests (unchanged behavior).
      {
        plugins: [
          viteTsConfigPaths({ projects: ["./tsconfig.json"] }),
          viteReact(),
        ],
        test: {
          name: "unit",
          environment: "jsdom",
          include: ["src/**/*.test.{ts,tsx}"],
          setupFiles: ["src/test/setup.ts"],
        },
      },
      // Project 2 — Storybook stories run as tests in a real Chromium via
      // Playwright. The plugin discovers stories from .storybook/main.ts.
      {
        plugins: [storybookTest({ configDir: ".storybook" })],
        test: {
          name: "storybook",
          browser: {
            enabled: true,
            provider: playwright(),
            headless: true,
            instances: [{ browser: "chromium" }],
          },
          // Since Storybook 10.3, @storybook/addon-vitest applies the preview
          // annotations (i18n + theme decorators) automatically — no setup
          // file needed.
        },
      },
    ],
  },
})

import type { StorybookConfig } from "@storybook/react-vite"

// The app's vite.config.ts registers TanStack Start, Nitro, the router
// generator, and the devtools plugin. Storybook auto-loads that config and
// merges those plugins into the base it hands `viteFinal`. They assume the
// full app runtime (a server, a route tree, an HTML entry of their own) and
// break a Storybook build. Strip them by name prefix while keeping
// Storybook's own internal plugins plus React, Tailwind, and tsconfig-paths.
const APP_ONLY_PLUGIN_PREFIXES = [
  "@tanstack/devtools:",
  "nitro:",
  "fullstack:",
  "tanstack-react-start:",
  "tanstack-start-core:",
  "tanstack-start:",
  "tanstack-router:",
  "tanstack:router-generator",
]

function pluginName(plugin: unknown): string | undefined {
  return plugin && typeof plugin === "object" && "name" in plugin
    ? (plugin as { name?: string }).name
    : undefined
}

const config: StorybookConfig = {
  stories: ["../src/**/*.stories.@(ts|tsx)"],
  addons: [
    "@storybook/addon-docs",
    "@storybook/addon-themes",
    "@storybook/addon-vitest",
    "@storybook/addon-mcp",
  ],
  framework: {
    name: "@storybook/react-vite",
    options: {},
  },
  core: {
    disableTelemetry: true,
  },
  viteFinal: async (base) => {
    // Under the Vitest browser runner (@storybook/addon-vitest), Storybook's
    // own dev plugins — chiefly storybook:optimize-deps-plugin — fight the
    // runner's dep handling and break the addon's setup-file import. There the
    // stories only need a minimal, conflict-free pipeline, so rebuild it from
    // scratch: React, tsconfig-paths (the `@/` alias), and Tailwind v4.
    if (process.env.VITEST) {
      const { mergeConfig } = await import("vite")
      const viteReact = (await import("@vitejs/plugin-react")).default
      const viteTsConfigPaths = (await import("vite-tsconfig-paths")).default
      const tailwindcss = (await import("@tailwindcss/vite")).default
      return mergeConfig(
        { ...base, plugins: [] },
        {
          plugins: [
            viteReact(),
            viteTsConfigPaths({ projects: ["./tsconfig.json"] }),
            tailwindcss(),
          ],
        },
      )
    }
    // Storybook dev/build: keep Storybook's internal plugins (the build needs
    // them) and drop only the app-only plugins. Type the list loosely as
    // unknown to dodge the deeply-recursive PluginOption type when flattening
    // preset arrays.
    const existing: Array<unknown> = base.plugins ?? []
    const kept = existing.flat(Infinity).filter((plugin) => {
      const name = pluginName(plugin)
      if (!name) return true
      return !APP_ONLY_PLUGIN_PREFIXES.some((prefix) => name.startsWith(prefix))
    })
    return { ...base, plugins: kept as NonNullable<typeof base.plugins> }
  },
}

export default config

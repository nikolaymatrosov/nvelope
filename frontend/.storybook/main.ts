import type { StorybookConfig } from "@storybook/react-vite"

const config: StorybookConfig = {
  stories: ["../src/**/*.stories.@(ts|tsx)"],
  addons: [
    "@storybook/addon-docs",
    "@storybook/addon-themes",
    "@storybook/addon-vitest",
  ],
  framework: {
    name: "@storybook/react-vite",
    options: {},
  },
  core: {
    disableTelemetry: true, // 👈 Disables telemetry
  },
  // The app's vite.config.ts registers TanStack Start, Nitro, and the
  // devtools plugin. Storybook auto-loads that config, but those plugins
  // assume the full app runtime and have no place in a component sandbox —
  // they slow or break the boot. Rebuild a minimal plugin set here instead:
  // React (JSX/Fast Refresh), tsconfig-paths (the `@/` alias), and Tailwind v4.
  viteFinal: async (base) => {
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
  },
}

export default config

import { defineConfig } from "vite"
import { devtools } from "@tanstack/devtools-vite"
import { tanstackStart } from "@tanstack/react-start/plugin/vite"
import viteReact from "@vitejs/plugin-react"
import viteTsConfigPaths from "vite-tsconfig-paths"
import tailwindcss from "@tailwindcss/vite"
import { nitro } from "nitro/vite"

const config = defineConfig({
  // Proxy API calls to the Go API service during development so the browser
  // talks to it same-origin and the session cookie just works.
  server: {
    proxy: {
      "/api": { target: "http://localhost:8080", changeOrigin: true },
      "^/t/[^/]+/api": { target: "http://localhost:8080", changeOrigin: true },
    },
  },
  plugins: [
    devtools(),
    nitro(),
    // this is the plugin that enables path aliases
    viteTsConfigPaths({
      projects: ["./tsconfig.json"],
    }),
    tailwindcss(),
    tanstackStart({
      router: {
        // Colocated test files live beside routes — keep them out of the tree.
        routeFileIgnorePattern: "\\.(test|spec)\\.",
      },
    }),
    viteReact(),
  ],
})

export default config

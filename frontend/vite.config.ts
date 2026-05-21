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
  //
  // Three paths are intentionally NOT proxied: Nitro intercepts them via the
  // file-based-routing handlers under src/server/routes/ before this proxy
  // sees them, because the BFF renders email-ready HTML server-side before
  // forwarding the saved doc to Go (see specs/014-visual-email-editor/
  // research.md § R4):
  //   - PUT  /t/:slug/api/campaigns/:id/visual
  //   - PUT  /t/:slug/api/templates/:id/visual
  //   - POST /t/:slug/api/render-preview
  // Everything else under /t/{slug}/api/* still proxies to Go transparently.
  server: {
    proxy: {
      "/api": { target: "http://localhost:8080", changeOrigin: true },
      "^/t/[^/]+/api(?!/campaigns/[^/]+/visual$|/templates/[^/]+/visual$|/render-preview$)":
        { target: "http://localhost:8080", changeOrigin: true },
    },
  },
  plugins: [
    devtools(),
    nitro({
      // Scan src/server/routes/ instead of the default ./routes/ so the BFF
      // file-based route handlers live next to the orchestrators they
      // delegate to.
      routesDir: "src/server/routes",
    }),
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

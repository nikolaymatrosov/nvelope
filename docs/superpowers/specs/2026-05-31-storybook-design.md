# Storybook for the frontend — Design

Date: 2026-05-31
Status: Approved (pending implementation plan)

## Goal

Add Storybook to `frontend/` to (1) develop and document UI components in
isolation and (2) run story-based visual/interaction tests in CI. The driving
use case is expanding the **visual-editor** feature set, so visual-editor
components get first-class story coverage.

## Context

`frontend/` is a TanStack Start + Vite 7 app: React 19, Tailwind v4 (via the
`@tailwindcss/vite` plugin), shadcn/ui (~25 primitives under
`src/components/ui/` plus `visual-editor/`, `shell/`, `settings/`, `billing/`,
`common/`), and i18next. Tests today run on Vitest + jsdom
(`vitest.config.ts`, `src/test/setup.ts`).

Two facts shape the design:

- **Dark mode is a plain `.dark` class.** `styles.css:6` defines
  `@custom-variant dark (&:is(.dark *))`. `next-themes` is in `package.json`
  but is **not** wired into `src` — no `ThemeProvider` is mounted. So theming
  in Storybook is just toggling the `.dark` class on the story root, not
  reproducing a provider.
- **i18n is a singleton.** Components call `useTranslation`; the existing
  Vitest setup imports `@/i18n` to initialize it. Stories need the same.

## Decisions

- **Storybook 9.x**, framework `@storybook/react-vite`. Storybook reuses the
  app's Vite pipeline (Tailwind v4, tsconfig path aliases) so stories render
  identically to the running app.
- **Visual testing via `@storybook/addon-vitest`**, registered as a **separate
  browser-mode Vitest project** (Playwright/Chromium) alongside the existing
  jsdom unit-test project. A single `pnpm test` runs both. Both run in CI.
- **Initial story scope: Seed + visual-editor.** A few shadcn primitives as
  pattern-setting seeds, then visual-editor component coverage.

## Architecture

### Dependencies (frontend devDependencies)

All Storybook packages pinned to the same 9.x minor:

- `storybook`
- `@storybook/react-vite`
- `@storybook/addon-vitest`
- `@storybook/addon-docs`
- `@storybook/addon-themes`
- `@vitest/browser`
- `playwright`
- `eslint-plugin-storybook`

### `.storybook/main.ts`

- `framework: "@storybook/react-vite"`.
- `stories: ["../src/**/*.stories.@(ts|tsx)"]`.
- `addons: ["@storybook/addon-docs", "@storybook/addon-themes",
  "@storybook/addon-vitest"]`.
- `viteFinal` hook: start from the app config but **drop the app-only plugins**
  that would break or slow a Storybook boot — TanStack Start, Nitro, and the
  devtools plugin. Keep React, `vite-tsconfig-paths`, and Tailwind. (Implement
  by constructing a minimal plugin set in `viteFinal` rather than importing the
  full `vite.config.ts`, to avoid the Start/Nitro plugins being instantiated.)

### `.storybook/preview.tsx`

- `import "../src/styles.css"` so Tailwind tokens and the `.dark` variant load.
- `withThemeByClassName` decorator (`@storybook/addon-themes`): themes
  `{ light: "", dark: "dark" }`, default `light`, target the story root —
  adds a toolbar toggle that flips the `.dark` class.
- i18n decorator: wrap each story in `<I18nextProvider i18n={i18n}>` using the
  initialized `@/i18n` singleton, so `useTranslation` renders real copy.
- Sensible `parameters` (layout, controls matchers) per Storybook defaults.

### Vitest integration

Convert `vitest.config.ts` to a `projects` array:

- **Project 1 — unit (unchanged behavior):** jsdom, `include:
  ["src/**/*.test.{ts,tsx}"]`, `setupFiles: ["src/test/setup.ts"]`, the
  existing `vite-tsconfig-paths` + React plugins.
- **Project 2 — storybook:** the `storybookTest` plugin from
  `@storybook/addon-vitest/vitest-plugin`, browser mode with provider
  `playwright`, `headless: true`, instance `chromium`. A
  `.storybook/vitest.setup.ts` applies the preview annotations
  (`setProjectAnnotations`).

`pnpm test` (i.e. `vitest run`) runs both projects.

### Scripts (`frontend/package.json`)

- `"storybook": "storybook dev -p 6006"`
- `"build-storybook": "storybook build"`
- `"test"` unchanged in name (`vitest run`) but now spans both projects.

### Stories (Seed + visual-editor)

- **Seed primitives** (establish the args/variants/play pattern):
  `ui/button.stories.tsx` (already has `button.test.tsx` to model against),
  plus `ui/badge.stories.tsx` and `ui/alert.stories.tsx`.
- **Visual-editor:** stories for the visual-editor components under
  `src/components/visual-editor/`, each with a `play` interaction test where it
  adds value (e.g. exercising a toolbar/plugin). These render the real editor
  UI in isolation and become the surface for building new editor features.

### ESLint

- Add `eslint-plugin-storybook`'s flat config to `eslint.config.js` so
  `*.stories.tsx` are linted under the existing `--max-warnings 0` gate.
- Ignore `storybook-static/` (build output) in lint/format/git as needed.

### CI (`.github/workflows/ci.yml`, `frontend` job)

- After `pnpm install`, add:
  `pnpm exec playwright install --with-deps chromium`.
- Existing `pnpm test` step now also runs story tests (no change to the step
  itself).
- Add a lightweight `pnpm build-storybook` smoke step to catch story/config
  breakage that the test run might not.

## Testing strategy

- Story interaction tests run via the Vitest browser project in CI (real
  Chromium), giving faithful rendering for visual-editor work.
- Existing jsdom unit tests are untouched and continue to run in the same
  `pnpm test` invocation.
- `build-storybook` acts as a compile/smoke gate for the whole story set.

## Out of scope

- Visual regression snapshot/diffing (e.g. Chromatic) — not requested.
- Wiring `next-themes` into the app.
- Stories for every shadcn primitive and every non-editor component area
  (shell/settings/billing/common) — those can be added incrementally later.

## Risks / notes

- Storybook's auto-loaded `vite.config.ts` includes TanStack Start + Nitro
  plugins; the `viteFinal` strip is the main integration risk and must be
  verified by booting Storybook locally.
- Playwright/Chromium adds CI install time; mitigated by the standard
  `--with-deps chromium` step and pnpm caching already configured.

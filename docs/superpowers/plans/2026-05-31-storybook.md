# Storybook for the frontend — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Storybook 10 to `frontend/` for isolated component development plus story-based interaction tests that run in CI, with first-class coverage of the visual-editor components.

**Architecture:** Storybook reuses the app's Vite pipeline (Tailwind v4, tsconfig path aliases) via `@storybook/react-vite`, with app-only plugins (TanStack Start, Nitro, devtools) stripped in a `viteFinal` hook. Theming is a `.dark`-class toolbar toggle (`@storybook/addon-themes`); i18n is provided by a decorator wrapping stories in the existing `@/i18n` singleton. `@storybook/addon-vitest` registers a browser-mode (Playwright/Chromium) Vitest project that runs alongside the existing jsdom unit-test project, so a single `pnpm test` runs both, in CI.

**Tech Stack:** Storybook 10.4.x, `@storybook/react-vite`, `@storybook/addon-vitest`, `@storybook/addon-docs`, `@storybook/addon-themes`, `@vitest/browser`, Playwright/Chromium, Vitest 3, React 19, Tailwind v4, i18next.

**Spec:** `docs/superpowers/specs/2026-05-31-storybook-design.md`

**Working directory:** All `pnpm`/`git` commands run **inside `frontend/`** unless noted. The first command shows `cd frontend`; subsequent calls assume the shell is already there (`cd` persists between Bash calls in this environment). All file paths below are relative to `frontend/`.

**Pinned versions:** Storybook packages are all `10.4.1` (current latest, verified via `npm view`). Pin them together — never mix Storybook majors/minors.

---

## File Structure

Files created:

- `frontend/.storybook/main.ts` — Storybook config: framework, stories glob, addons, `viteFinal` plugin strip.
- `frontend/.storybook/preview.tsx` — global CSS import, theme decorator, i18n decorator, parameters.
- `frontend/.storybook/vitest.setup.ts` — applies preview annotations to the Vitest browser project.
- `frontend/src/components/ui/button.stories.tsx` — seed primitive story + play test.
- `frontend/src/components/ui/badge.stories.tsx` — seed primitive story.
- `frontend/src/components/ui/alert.stories.tsx` — seed primitive story.
- `frontend/src/components/visual-editor/ui/ThemeControls.stories.tsx` — visual-editor story + play interaction test.
- `frontend/src/components/visual-editor/ui/MergeTagPicker.stories.tsx` — visual-editor story with QueryClient decorator + play filter test.

Files modified:

- `frontend/package.json` — devDependencies + `storybook` / `build-storybook` scripts.
- `frontend/vitest.config.ts` — convert to a `projects` array (jsdom unit project + Storybook browser project).
- `frontend/eslint.config.js` — add `eslint-plugin-storybook` flat config; ignore `storybook-static`.
- `frontend/.gitignore` (or repo `.gitignore`) — ignore `storybook-static/`.
- `.github/workflows/ci.yml` — Playwright install step + `build-storybook` smoke step.

---

## Task 1: Install Storybook and get a baseline boot

**Files:**
- Create: `frontend/.storybook/main.ts`, `frontend/.storybook/preview.tsx` (via init scaffold, refined in later tasks)
- Modify: `frontend/package.json`

- [ ] **Step 1: Run the Storybook initializer**

The initializer detects React + Vite, scaffolds `.storybook/`, adds the `storybook` / `build-storybook` scripts, installs `storybook` + `@storybook/react-vite` + `@storybook/addon-docs`, and (detecting the existing Vitest) offers the Vitest addon. Accept the Vitest addon and the Playwright install when prompted.

Run:
```bash
cd frontend && pnpm dlx storybook@10.4.1 init --package-manager pnpm
```

Expected: `.storybook/main.ts` and `.storybook/preview.ts(x)` created; `package.json` gains `"storybook"` and `"build-storybook"` scripts and Storybook devDependencies; `@storybook/addon-vitest`, `@vitest/browser`, and `playwright` added; a `.storybook/vitest.setup.ts` and a Vitest config/project file created or updated. If init opens a browser, close it.

- [ ] **Step 2: Pin all Storybook packages to 10.4.1**

Open `package.json` and ensure every `storybook` / `@storybook/*` devDependency is exactly `10.4.1` (no `^`). Then:

Run:
```bash
pnpm install
```

Expected: install completes, lockfile updated.

- [ ] **Step 3: Verify Storybook boots**

Run:
```bash
pnpm storybook --ci --quiet & SB_PID=$!; sleep 25; curl -sSf http://localhost:6006/index.json > /dev/null && echo "SB_OK"; kill $SB_PID
```

Expected: prints `SB_OK` (the dev server served the story index). If it fails because app-only Vite plugins (TanStack Start/Nitro) crash the boot, that is fixed in Task 2 — proceed to Task 2 and re-run this verification there.

- [ ] **Step 4: Commit**

```bash
git add frontend/.storybook frontend/package.json frontend/pnpm-lock.yaml frontend/vitest* .gitignore frontend/.gitignore 2>/dev/null; git commit -m "chore(frontend): scaffold Storybook 10 with vitest addon"
```

---

## Task 2: Strip app-only Vite plugins via `viteFinal`

The app's `vite.config.ts` is auto-discovered by `@storybook/react-vite` and pulls in TanStack Start, Nitro, and the devtools plugin — none of which belong in a component sandbox and which can break or slow the boot. Replace whatever init produced in `main.ts` with an explicit config that keeps only React, tsconfig-paths, and Tailwind.

**Files:**
- Modify: `frontend/.storybook/main.ts`

- [ ] **Step 1: Write `main.ts`**

Replace the entire file with:

```ts
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
```

- [ ] **Step 2: Add `@storybook/addon-themes` (referenced above)**

Run:
```bash
pnpm add -D @storybook/addon-themes@10.4.1
```

Expected: package added.

- [ ] **Step 3: Verify Storybook boots clean**

Run:
```bash
pnpm storybook --ci --quiet & SB_PID=$!; sleep 25; curl -sSf http://localhost:6006/index.json > /dev/null && echo "SB_OK"; kill $SB_PID
```

Expected: prints `SB_OK` with no TanStack Start / Nitro errors in the captured output.

- [ ] **Step 4: Commit**

```bash
git add frontend/.storybook/main.ts frontend/package.json frontend/pnpm-lock.yaml
git commit -m "chore(frontend): keep only react/tailwind/paths in Storybook vite config"
```

---

## Task 3: Theme decorator (light/dark toolbar toggle)

Dark mode is the `.dark` class (`styles.css:6` → `@custom-variant dark (&:is(.dark *))`). `withThemeByClassName` adds a toolbar switch that toggles that class on the story root.

**Files:**
- Modify: `frontend/.storybook/preview.tsx`

- [ ] **Step 1: Write `preview.tsx` with the global CSS import and theme decorator**

Replace the entire file with:

```tsx
import { withThemeByClassName } from "@storybook/addon-themes"
import type { Preview } from "@storybook/react-vite"

// Load Tailwind tokens + the `.dark` variant so stories render exactly like
// the app.
import "../src/styles.css"

const preview: Preview = {
  parameters: {
    controls: {
      matchers: { color: /(background|color)$/i, date: /Date$/i },
    },
    layout: "centered",
  },
  decorators: [
    withThemeByClassName({
      themes: { light: "", dark: "dark" },
      defaultTheme: "light",
    }),
  ],
}

export default preview
```

- [ ] **Step 2: Verify the toolbar toggle renders**

Run:
```bash
pnpm storybook --ci --quiet & SB_PID=$!; sleep 25; curl -sSf http://localhost:6006/index.json > /dev/null && echo "SB_OK"; kill $SB_PID
```

Expected: `SB_OK`. (Visual confirmation of the toggle happens once stories exist in Task 5.)

- [ ] **Step 3: Commit**

```bash
git add frontend/.storybook/preview.tsx
git commit -m "chore(frontend): add Storybook light/dark theme toolbar"
```

---

## Task 4: i18n decorator

Components call `useTranslation`. Wrap every story in the initialized `@/i18n` singleton (the same instance the app and the Vitest unit setup use) so stories render real copy instead of raw keys.

**Files:**
- Modify: `frontend/.storybook/preview.tsx`

- [ ] **Step 1: Add the i18n decorator**

Edit `preview.tsx`. Add these imports at the top (after the existing imports):

```tsx
import { I18nextProvider } from "react-i18next"
import i18n from "@/i18n"
```

Then add the i18n decorator to the `decorators` array so the file's `decorators` reads:

```tsx
  decorators: [
    (Story) => (
      <I18nextProvider i18n={i18n}>
        <Story />
      </I18nextProvider>
    ),
    withThemeByClassName({
      themes: { light: "", dark: "dark" },
      defaultTheme: "light",
    }),
  ],
```

(Decorators apply bottom-up; the theme wrapper stays innermost, i18n outermost — order is not significant here, but keep i18n first so every story, including the theme-wrapped tree, has translations.)

- [ ] **Step 2: Verify boot still works**

Run:
```bash
pnpm storybook --ci --quiet & SB_PID=$!; sleep 25; curl -sSf http://localhost:6006/index.json > /dev/null && echo "SB_OK"; kill $SB_PID
```

Expected: `SB_OK`.

- [ ] **Step 3: Commit**

```bash
git add frontend/.storybook/preview.tsx
git commit -m "chore(frontend): provide i18n to all stories"
```

---

## Task 5: Seed primitive stories (Button, Badge, Alert)

Establish the args/variants/play-test pattern with three shadcn primitives. The Button story includes a `play` interaction test that doubles as the first browser-mode test exercised in Task 6.

**Files:**
- Create: `frontend/src/components/ui/button.stories.tsx`
- Create: `frontend/src/components/ui/badge.stories.tsx`
- Create: `frontend/src/components/ui/alert.stories.tsx`

- [ ] **Step 1: Write `button.stories.tsx` (with a play test)**

```tsx
import { expect, fn, userEvent, within } from "storybook/test"
import { Button } from "./button"
import type { Meta, StoryObj } from "@storybook/react-vite"

const meta = {
  title: "UI/Button",
  component: Button,
  args: { children: "Button", onClick: fn() },
  argTypes: {
    variant: {
      control: "select",
      options: [
        "default",
        "outline",
        "secondary",
        "ghost",
        "destructive",
        "link",
      ],
    },
    size: {
      control: "select",
      options: ["default", "xs", "sm", "lg", "icon"],
    },
  },
} satisfies Meta<typeof Button>

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = {}

export const Destructive: Story = {
  args: { variant: "destructive", children: "Delete" },
}

export const Secondary: Story = { args: { variant: "secondary" } }

export const Clicks: Story = {
  play: async ({ args, canvasElement }) => {
    const canvas = within(canvasElement)
    const button = canvas.getByRole("button", { name: "Button" })
    await userEvent.click(button)
    await expect(args.onClick).toHaveBeenCalledTimes(1)
  },
}
```

- [ ] **Step 2: Write `badge.stories.tsx`**

```tsx
import { Badge } from "./badge"
import type { Meta, StoryObj } from "@storybook/react-vite"

const meta = {
  title: "UI/Badge",
  component: Badge,
  args: { children: "Badge" },
  argTypes: {
    variant: {
      control: "select",
      options: [
        "default",
        "secondary",
        "destructive",
        "outline",
        "ghost",
        "link",
      ],
    },
  },
} satisfies Meta<typeof Badge>

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = {}
export const Secondary: Story = { args: { variant: "secondary" } }
export const Destructive: Story = {
  args: { variant: "destructive", children: "Error" },
}
```

- [ ] **Step 3: Write `alert.stories.tsx`**

```tsx
import { Alert, AlertDescription, AlertTitle } from "./alert"
import type { Meta, StoryObj } from "@storybook/react-vite"

const meta = {
  title: "UI/Alert",
  component: Alert,
} satisfies Meta<typeof Alert>

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = {
  render: (args) => (
    <Alert {...args}>
      <AlertTitle>Heads up</AlertTitle>
      <AlertDescription>
        This is what an alert looks like in the sandbox.
      </AlertDescription>
    </Alert>
  ),
}
```

- [ ] **Step 4: Verify the stories load in Storybook**

Run:
```bash
pnpm storybook --ci --quiet & SB_PID=$!; sleep 25; curl -sSf http://localhost:6006/index.json | grep -q '"ui-button"' && echo "STORIES_OK"; kill $SB_PID
```

Expected: prints `STORIES_OK` (Button stories are indexed).

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/ui/button.stories.tsx frontend/src/components/ui/badge.stories.tsx frontend/src/components/ui/alert.stories.tsx
git commit -m "test(frontend): seed Storybook stories for Button, Badge, Alert"
```

---

## Task 6: Merge Vitest projects so `pnpm test` runs stories in the browser

Convert the single-config `vitest.config.ts` into a two-project config: the existing jsdom unit tests, plus the Storybook browser project. `pnpm test` then runs both. This is the core visual-testing wiring.

**Files:**
- Modify: `frontend/vitest.config.ts`
- Verify/Modify: `frontend/.storybook/vitest.setup.ts` (created by init in Task 1)

- [ ] **Step 1: Ensure `.storybook/vitest.setup.ts` applies preview annotations**

The file must read exactly:

```ts
import { beforeAll } from "vitest"
import { setProjectAnnotations } from "@storybook/react-vite"
import * as projectAnnotations from "./preview"

// Applies this project's global decorators (i18n + theme) and parameters to
// every story run as a test.
const project = setProjectAnnotations([projectAnnotations])

beforeAll(project.beforeAll)
```

If init created it under a different name or content, overwrite it to match.

- [ ] **Step 2: Rewrite `vitest.config.ts` as a `projects` array**

Replace the entire file with:

```ts
import { defineConfig } from "vitest/config"
import viteReact from "@vitejs/plugin-react"
import viteTsConfigPaths from "vite-tsconfig-paths"
import { storybookTest } from "@storybook/addon-vitest/vitest-plugin"

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
            provider: "playwright",
            headless: true,
            instances: [{ browser: "chromium" }],
          },
          setupFiles: [".storybook/vitest.setup.ts"],
        },
      },
    ],
  },
})
```

- [ ] **Step 3: Install the Chromium browser for Playwright (local)**

Run:
```bash
pnpm exec playwright install chromium
```

Expected: Chromium downloads (or "is already installed").

- [ ] **Step 4: Run the full suite — both projects must run and pass**

Run:
```bash
pnpm test
```

Expected: Vitest reports two projects, `unit` and `storybook`. The `storybook` project runs the Button/Badge/Alert stories (including the `Clicks` play test) in Chromium; the `unit` project runs the existing `*.test.tsx`. All pass.

- [ ] **Step 5: Commit**

```bash
git add frontend/vitest.config.ts frontend/.storybook/vitest.setup.ts
git commit -m "test(frontend): run Storybook stories as browser tests via vitest"
```

---

## Task 7: Visual-editor stories

The focus area. `ThemeControls` is fully presentational (props in, `onChange` out) — ideal for a worked story plus a `play` interaction test. `MergeTagPicker` needs a `QueryClient`; its data is seeded into the cache so the story is deterministic and network-free.

**Files:**
- Create: `frontend/src/components/visual-editor/ui/ThemeControls.stories.tsx`
- Create: `frontend/src/components/visual-editor/ui/MergeTagPicker.stories.tsx`

- [ ] **Step 1: Write `ThemeControls.stories.tsx` (with a pin/edit/reset play test)**

```tsx
import { useState } from "react"
import { expect, userEvent, within } from "storybook/test"
import { ThemeControls } from "./ThemeControls"
import type { Theme } from "@/lib/api-types"
import type { Meta, StoryObj } from "@storybook/react-vite"

const resolved: Theme = {
  textColor: "#111827",
  linkColor: "#0066cc",
  buttonColor: "#0066cc",
  buttonTextColor: "#ffffff",
  fontFamily: "Inter, sans-serif",
  containerWidth: 600,
}

// Stateful wrapper so the override round-trips through onChange like it does
// in the real editor chrome.
function Harness({ initial }: { initial: Theme | null }) {
  const [value, setValue] = useState<Theme | null>(initial)
  return <ThemeControls value={value} resolved={resolved} onChange={setValue} />
}

const meta = {
  title: "Visual Editor/ThemeControls",
  component: ThemeControls,
} satisfies Meta<typeof ThemeControls>

export default meta
type Story = StoryObj<typeof meta>

// Inheriting tenant branding (value === null).
export const Inheriting: Story = {
  render: () => <Harness initial={null} />,
}

// A pinned override with editable fields.
export const Pinned: Story = {
  render: () => <Harness initial={resolved} />,
}

// Pin → the editable body appears; reset → back to the inherit badge.
export const PinThenReset: Story = {
  render: () => <Harness initial={null} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement)
    await expect(
      canvas.getByTestId("ve-theme-inherit-badge"),
    ).toBeInTheDocument()
    await userEvent.click(canvas.getByTestId("ve-theme-pin-override"))
    await expect(
      canvas.getByTestId("ve-theme-pinned-body"),
    ).toBeInTheDocument()
    await userEvent.click(canvas.getByTestId("ve-theme-reset-defaults"))
    await expect(
      canvas.getByTestId("ve-theme-inherit-badge"),
    ).toBeInTheDocument()
  },
}
```

- [ ] **Step 2: Write `MergeTagPicker.stories.tsx` (QueryClient decorator + filter play test)**

```tsx
import { useState } from "react"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { expect, userEvent, within } from "storybook/test"
import { MergeTagPicker } from "./MergeTagPicker"
import { queryKeys } from "@/lib/query"
import type { MergeTagsResponse } from "@/lib/api-types"
import type { Meta, StoryObj } from "@storybook/react-vite"

const SLUG = "acme"

const data: MergeTagsResponse = {
  subscriber: [
    { slug: "first_name", displayName: "First name", type: "text", builtIn: true },
    { slug: "email", displayName: "Email", type: "text", builtIn: true },
  ],
  campaign: [{ key: "subject", displayName: "Subject" }],
}

// Seed the cache so useQuery resolves synchronously with no network — the
// picker reads queryKeys.mergeTags(SLUG).
function withSeededQuery(Story: () => React.ReactElement) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  client.setQueryData(queryKeys.mergeTags(SLUG), data)
  return <QueryClientProvider client={client}>{Story()}</QueryClientProvider>
}

// The picker is controlled-open here so it renders without an editor instance.
function Harness() {
  const [open, setOpen] = useState(true)
  return (
    <MergeTagPicker slug={SLUG} editor={null} open={open} onOpenChange={setOpen} />
  )
}

const meta = {
  title: "Visual Editor/MergeTagPicker",
  component: MergeTagPicker,
  decorators: [withSeededQuery],
} satisfies Meta<typeof MergeTagPicker>

export default meta
type Story = StoryObj<typeof meta>

export const Open: Story = {
  render: () => <Harness />,
}

// Typing in the filter narrows the subscriber list.
export const Filtering: Story = {
  render: () => <Harness />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement)
    await expect(
      canvas.getByTestId("ve-merge-tag-item-subscriber-first_name"),
    ).toBeInTheDocument()
    await userEvent.type(canvas.getByTestId("ve-merge-tag-filter"), "email")
    await expect(
      canvas.getByTestId("ve-merge-tag-item-subscriber-email"),
    ).toBeInTheDocument()
    await expect(
      canvas.queryByTestId("ve-merge-tag-item-subscriber-first_name"),
    ).not.toBeInTheDocument()
  },
}
```

- [ ] **Step 3: Run the suite — new story tests must pass in Chromium**

Run:
```bash
pnpm test
```

Expected: `storybook` project now includes the ThemeControls `PinThenReset` and MergeTagPicker `Filtering` play tests; all pass. `unit` project unchanged.

- [ ] **Step 4: Typecheck (stories are type-checked too)**

Run:
```bash
pnpm typecheck
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/visual-editor/ui/ThemeControls.stories.tsx frontend/src/components/visual-editor/ui/MergeTagPicker.stories.tsx
git commit -m "test(frontend): add Storybook stories for ThemeControls and MergeTagPicker"
```

---

## Task 8: ESLint for stories + ignore build output

Stories must pass the existing `--max-warnings 0` lint gate, and `eslint-plugin-storybook` enforces story best practices.

**Files:**
- Modify: `frontend/eslint.config.js`
- Modify: `frontend/.gitignore` (create if absent)

- [ ] **Step 1: Install the ESLint plugin**

Run:
```bash
pnpm add -D eslint-plugin-storybook@10.4.1
```

Expected: package added.

- [ ] **Step 2: Read the current `eslint.config.js`**

Run:
```bash
cat eslint.config.js
```

Expected: a flat-config array export. Note how configs are composed (spread vs array entries) and any existing `ignores`.

- [ ] **Step 3: Add the Storybook flat config and ignore `storybook-static`**

At the top of `eslint.config.js`, add the import:

```js
import storybook from "eslint-plugin-storybook"
```

Append `...storybook.configs["flat/recommended"]` to the exported config array (the plugin's flat preset already scopes its rules to `*.stories.@(ts|tsx)` and `.storybook/`). Add a global ignore entry for the build output if one is not already present:

```js
{ ignores: ["storybook-static/**"] }
```

- [ ] **Step 4: Ignore the Storybook build output in git**

Add the line `storybook-static/` to `frontend/.gitignore` (create the file with just that line if it does not exist).

- [ ] **Step 5: Run lint**

Run:
```bash
pnpm lint
```

Expected: exits 0 with zero warnings. Fix any story-rule violations the plugin reports (e.g. add missing `export default meta`), then re-run until clean.

- [ ] **Step 6: Commit**

```bash
git add frontend/eslint.config.js frontend/package.json frontend/pnpm-lock.yaml frontend/.gitignore
git commit -m "chore(frontend): lint Storybook stories and ignore build output"
```

---

## Task 9: CI — Playwright install + build-storybook smoke

The `frontend` CI job already runs `pnpm test`, which now includes the browser project. CI needs Chromium installed for that to pass, plus a `build-storybook` smoke step to catch story/config breakage.

**Files:**
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Read the current frontend job**

Run (from repo root):
```bash
sed -n '36,84p' ../.github/workflows/ci.yml
```

Expected: the `frontend` job with `Install` (line ~57) then `Typecheck`/`Lint`/`Test`/`Build` steps.

- [ ] **Step 2: Add a Playwright install step after `Install`**

Insert this step immediately after the `Install` step (`run: pnpm install --frozen-lockfile`):

```yaml
      - name: Install Playwright Chromium
        run: pnpm exec playwright install --with-deps chromium
```

(`--with-deps` installs the OS libraries Chromium needs on `ubuntu-latest`.)

- [ ] **Step 3: Add a build-storybook smoke step after `Build`**

Append this step at the end of the `frontend` job's `steps` (after the existing `Build` step):

```yaml
      - name: Build Storybook
        run: pnpm build-storybook
```

- [ ] **Step 4: Validate the workflow YAML**

Run (from repo root):
```bash
python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/ci.yml')); print('YAML_OK')"
```

Expected: prints `YAML_OK`.

- [ ] **Step 5: Locally reproduce the CI test + build path**

Run (inside `frontend/`):
```bash
pnpm test && pnpm build-storybook
```

Expected: tests pass (both projects) and `storybook build` completes, emitting `storybook-static/`.

- [ ] **Step 6: Commit**

```bash
git add ../.github/workflows/ci.yml
git commit -m "ci(frontend): install Chromium and smoke-build Storybook"
```

---

## Final verification

- [ ] **Full gate, mirroring CI:**

Run (inside `frontend/`):
```bash
pnpm typecheck && pnpm lint && pnpm test && pnpm build && pnpm build-storybook
```

Expected: every command exits 0. Story interaction tests run in Chromium under `pnpm test`; `pnpm storybook` serves the sandbox for local development.

---

## Self-Review notes

- **Spec coverage:** deps (Task 1,2,8), `.storybook/main.ts` viteFinal strip (Task 2), preview theme+i18n decorators (Task 3,4), Vitest two-project merge (Task 6), scripts (Task 1), seed stories (Task 5), visual-editor stories (Task 7), ESLint (Task 8), CI Playwright + build-storybook smoke (Task 9). All spec sections map to a task.
- **Version consistency:** all `@storybook/*` and `storybook` pinned to `10.4.1` (Task 1 Step 2; explicit `@10.4.1` on every `pnpm add`).
- **Type/name consistency:** `Theme` fields (textColor/linkColor/buttonColor/buttonTextColor/fontFamily/containerWidth) match `api-types.ts:812`; `MergeTagsResponse`/`MergeTagSubscriberItem` (`slug`,`displayName`,`type`,`builtIn`) and `MergeTagCampaignItem` (`key`,`displayName`) match `api-types.ts:863`; `queryKeys.mergeTags(slug)` matches `query.ts:123`; ThemeControls/MergeTagPicker `data-testid`s match the component source. `setProjectAnnotations` imported from `@storybook/react-vite` in both `vitest.setup.ts` and per the framework package.
- **Known risk:** the `viteFinal` strip (Task 2) is the main integration risk; its verification step boots Storybook and asserts a served index.
```

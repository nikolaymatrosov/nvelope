import { I18nextProvider } from "react-i18next"
import { withThemeByClassName } from "@storybook/addon-themes"
import type { Preview } from "@storybook/react-vite"

import i18n from "@/i18n"

// Load Tailwind tokens + the `.dark` variant so stories render exactly like
// the app.
import "../src/styles.css"

// Collapse enter/exit animations to zero under the Vitest browser runner.
//
// The portalled overlays (dialog, alert-dialog, sheet, dropdown) open with
// `data-open:animate-in fade-in-0`, whose keyframes start at `opacity: 0` on
// the *content* element. jest-dom's `toBeVisible()` walks the ancestor chain,
// so a story that asserts visibility right after open fails whenever the
// assertion lands inside the first frames of that animation. That is a pure
// timing race: it never reproduces on a warm Vite cache and reproduces on
// roughly half of cold-cache runs, where transform work delays the first
// animation frame past the assertion.
//
// Zeroing the durations removes the race without touching a single assertion —
// the stories still require the overlay to be genuinely visible. Real
// Storybook (dev and `build-storybook`) keeps its animations.
if (import.meta.env.VITEST_STORYBOOK) {
  const style = document.createElement("style")
  style.textContent = `*, *::before, *::after {
    animation-duration: 0s !important;
    animation-delay: 0s !important;
    transition-duration: 0s !important;
    transition-delay: 0s !important;
  }`
  document.head.appendChild(style)
}

const preview: Preview = {
  parameters: {
    controls: {
      matchers: { color: /(background|color)$/i, date: /Date$/i },
    },
    layout: "centered",
  },
  decorators: [
    // i18n outermost so every story — including the theme-wrapped tree — has
    // real translations instead of raw keys.
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
}

export default preview

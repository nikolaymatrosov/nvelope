import { I18nextProvider } from "react-i18next"
import { withThemeByClassName } from "@storybook/addon-themes"
import type { Preview } from "@storybook/react-vite"

import i18n from "@/i18n"

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

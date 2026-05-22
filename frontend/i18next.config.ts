import { defineConfig } from "i18next-cli"

// i18next-cli config — drives `pnpm i18n:types` (typed-resource generation) and
// `pnpm i18n:lint` (hardcoded-string detection). English is the primary
// language and the source of truth for keys; Russian is secondary.
export default defineConfig({
  locales: ["en", "ru"],
  extract: {
    input: ["src/**/*.{ts,tsx}"],
    // Test files assert on literal copy — they are not UI surfaces and must
    // not be linted for hardcoded strings.
    ignore: ["src/**/*.test.{ts,tsx}"],
    output: "src/locales/{{language}}/{{namespace}}.json",
    defaultNS: "common",
    primaryLanguage: "en",
    secondaryLanguages: ["ru"],
    removeUnusedKeys: false,
    sort: true,
    indentation: 2,
  },
  types: {
    input: ["src/locales/en/*.json"],
    // output is the `react-i18next`/`i18next` module-augmentation file;
    // resourcesFile holds the generated Resources interface it references.
    output: "src/i18n/i18next.d.ts",
    resourcesFile: "src/i18n/resources.d.ts",
  },
})

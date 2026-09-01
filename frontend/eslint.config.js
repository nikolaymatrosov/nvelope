//  @ts-check

import { tanstackConfig } from "@tanstack/eslint-config"
import storybook from "eslint-plugin-storybook"

export default [
  {
    ignores: [
      ".output/**",
      ".nitro/**",
      "dist/**",
      "storybook-static/**",
      // Emitted by `make generate` from api/openapi/. Never hand-edited, so
      // house style (e.g. Array<T> over T[]) is not ours to enforce here.
      "src/lib/api-schema.gen.ts",
    ],
  },
  ...tanstackConfig,
  // Storybook best-practice rules, scoped by the preset to *.stories.* and
  // the .storybook/ config directory.
  ...storybook.configs["flat/recommended"],
]

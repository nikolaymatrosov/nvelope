//  @ts-check

import { tanstackConfig } from "@tanstack/eslint-config"
import storybook from "eslint-plugin-storybook"

export default [
  { ignores: [".output/**", ".nitro/**", "dist/**", "storybook-static/**"] },
  ...tanstackConfig,
  // Storybook best-practice rules, scoped by the preset to *.stories.* and
  // the .storybook/ config directory.
  ...storybook.configs["flat/recommended"],
]

// Renders the react-email templates to the static HTML + plain-text + subject
// artifacts that the Go verification worker embeds. Run via `pnpm emails:build`.
//
// Templates are driven with placeholder props ({{name}}, {{verifyUrl}}); the Go
// worker substitutes the real values per recipient at send time. CI re-runs
// this script and fails on a dirty git tree, so the committed artifacts under
// internal/auth/adapters/emails/ always match the templates.

import { mkdir, writeFile } from "node:fs/promises"
import { dirname, join } from "node:path"
import { fileURLToPath } from "node:url"

import { render } from "react-email"

import { VerifyEmail, verifyEmailSubject } from "@/emails/verify-email"

// Placeholder sentinels substituted by the Go worker. They carry no
// HTML-special characters, so they survive react-email rendering verbatim.
const NAME = "{{name}}"
const VERIFY_URL = "{{verifyUrl}}"

const LOCALES = ["en", "ru"] as const

// One entry per email template — adding a template is one entry here.
const TEMPLATES = [
  {
    slug: "verify-email",
    subject: verifyEmailSubject,
    element: (locale: string) => (
      <VerifyEmail name={NAME} verifyUrl={VERIFY_URL} locale={locale} />
    ),
  },
]

const scriptDir = dirname(fileURLToPath(import.meta.url))
const outDir = join(scriptDir, "../../internal/auth/adapters/emails")

await mkdir(outDir, { recursive: true })

for (const tpl of TEMPLATES) {
  for (const locale of LOCALES) {
    const element = tpl.element(locale)
    const html = await render(element, { pretty: true })
    const text = await render(element, { plainText: true })
    const subject = tpl.subject(locale)

    const base = join(outDir, `${tpl.slug}.${locale}`)
    await writeFile(`${base}.subject.txt`, `${subject}\n`, "utf8")
    await writeFile(`${base}.html`, html, "utf8")
    await writeFile(`${base}.txt`, text, "utf8")
    process.stdout.write(`rendered ${tpl.slug} [${locale}]\n`)
  }
}

import {
  HeadContent,
  Link,
  Outlet,
  Scripts,
  createRootRoute,
} from "@tanstack/react-router"
import { TanStackRouterDevtoolsPanel } from "@tanstack/react-router-devtools"
import { TanStackDevtools } from "@tanstack/react-devtools"
import { QueryClientProvider } from "@tanstack/react-query"
import { I18nextProvider, useTranslation } from "react-i18next"
import { useEffect } from "react"

import appCss from "../styles.css?url"
import type { Locale } from "@/i18n/config"
import { queryClient } from "@/lib/query"
import { Toaster } from "@/components/ui/sonner"
import { Button } from "@/components/ui/button"
import i18n from "@/i18n"
import { DEFAULT_LOCALE, localeDir } from "@/i18n/config"
import { getServerLocale } from "@/i18n/server-locale"

export const Route = createRootRoute({
  // On the server, resolve the request locale and seed i18next before the
  // first render so SSR output is already in the user's language. The result
  // is hydrated to the client, so this server function runs only once.
  beforeLoad: async () => {
    if (import.meta.env.SSR) {
      const locale = await getServerLocale()
      await i18n.changeLanguage(locale)
      return { locale }
    }
    return { locale: i18n.language }
  },
  head: () => ({
    meta: [
      {
        charSet: "utf-8",
      },
      {
        name: "viewport",
        content: "width=device-width, initial-scale=1",
      },
      {
        title: "nvelope",
      },
    ],
    links: [
      {
        rel: "stylesheet",
        href: appCss,
      },
    ],
  }),
  notFoundComponent: () => (
    <main className="mx-auto flex min-h-svh max-w-md flex-col items-center justify-center gap-4 p-6 text-center">
      <h1 className="text-2xl font-semibold">Page not found</h1>
      <p className="text-sm text-muted-foreground">
        The page you were looking for does not exist.
      </p>
      <Button asChild>
        <Link to="/">Back home</Link>
      </Button>
    </main>
  ),
  component: RootComponent,
  shellComponent: RootDocument,
})

// Keeps the document's lang/dir attributes in sync with the active locale
// (FR-011) so assistive technology and the browser identify the language
// correctly. Renders nothing.
function HtmlLangSync() {
  const { i18n: instance } = useTranslation()

  useEffect(() => {
    const apply = (lng: string) => {
      const locale = (lng in localeDir ? lng : "en") as Locale
      document.documentElement.lang = locale
      document.documentElement.dir = localeDir[locale]
    }
    apply(instance.language)
    instance.on("languageChanged", apply)
    return () => instance.off("languageChanged", apply)
  }, [instance])

  return null
}

function RootComponent() {
  return (
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={queryClient}>
        <HtmlLangSync />
        <Outlet />
        <Toaster richColors position="bottom-right" />
      </QueryClientProvider>
    </I18nextProvider>
  )
}

function RootDocument({ children }: { children: React.ReactNode }) {
  // i18next is seeded server-side in the root beforeLoad, so its language is
  // correct for the SSR shell render here.
  const locale = (
    i18n.language in localeDir ? i18n.language : DEFAULT_LOCALE
  ) as Locale

  return (
    <html lang={locale} dir={localeDir[locale]}>
      <head>
        <HeadContent />
      </head>
      <body>
        {children}
        <TanStackDevtools
          config={{
            position: "bottom-right",
          }}
          plugins={[
            {
              name: "Tanstack Router",
              render: <TanStackRouterDevtoolsPanel />,
            },
          ]}
        />
        <Scripts />
      </body>
    </html>
  )
}

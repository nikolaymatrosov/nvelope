import { Link, createFileRoute } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { ArrowLeftIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { LanguageSelect } from "@/components/settings/language-select"
import { useSyncAccountLocale } from "@/hooks/use-locale"

export const Route = createFileRoute("/account/")({
  component: AccountView,
})

export function AccountView() {
  const { t } = useTranslation("account")
  // Apply the signed-in user's stored language preference (FR-004).
  useSyncAccountLocale()

  return (
    <main className="mx-auto flex w-full max-w-2xl flex-col gap-6 p-6">
      <div>
        <Button asChild variant="ghost" size="sm" className="mb-2 -ml-2">
          <Link to="/">
            <ArrowLeftIcon />
            {t("back")}
          </Link>
        </Button>
        <h1 className="text-2xl font-semibold">{t("title")}</h1>
        <p className="text-sm text-muted-foreground">{t("description")}</p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{t("language.title")}</CardTitle>
          <CardDescription>{t("language.cardDescription")}</CardDescription>
        </CardHeader>
        <CardContent>
          <LanguageSelect />
        </CardContent>
      </Card>
    </main>
  )
}

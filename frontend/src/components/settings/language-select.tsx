// The interface-language switcher. Selecting a language applies it in place
// (no reload, no URL change) via useLocale.

import { useTranslation } from "react-i18next"

import type { Locale } from "@/i18n/config"
import { useLocale } from "@/hooks/use-locale"
import { localeLabel } from "@/i18n/config"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

export function LanguageSelect() {
  const { t } = useTranslation("account")
  const { locale, setLocale, supportedLocales } = useLocale()

  return (
    <div className="flex flex-col gap-2">
      <Label htmlFor="language-select">{t("language.label")}</Label>
      <Select
        value={locale}
        onValueChange={(value) => void setLocale(value as Locale)}
      >
        <SelectTrigger id="language-select" className="w-60">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {supportedLocales.map((loc) => (
            <SelectItem key={loc} value={loc}>
              {localeLabel[loc]}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <p className="text-sm text-muted-foreground">{t("language.hint")}</p>
    </div>
  )
}

// The registration email-verification message, as a react-email template.
// Copy is translatable: every string comes from the i18next `emails` catalog,
// selected by the recipient's locale so the email matches the language the
// account was created in.

import {
  Body,
  Button,
  Container,
  Head,
  Html,
  Img,
  Link,
  Preview,
  Section,
  Tailwind,
  Text,
} from "react-email"

import i18n from "@/i18n/emails"
import { DEFAULT_LOCALE } from "@/i18n/config"

// The product name is a proper noun, identical across locales, so the logo's
// alt text is not translatable copy.
const BRAND_NAME = "nvelope"

interface VerifyEmailProps {
  name: string
  verifyUrl: string
  // locale selects the language; an unsupported value falls back to English.
  locale: string
}

// emailLocale narrows an arbitrary locale string to a supported catalog
// language, defaulting to English.
function emailLocale(locale: string): string {
  return locale === "ru" ? "ru" : DEFAULT_LOCALE
}

// verifyEmailSubject is the localized Subject header for the verification
// email. It lives outside the HTML body, so the sender composes it directly.
export function verifyEmailSubject(locale: string): string {
  return i18n.getFixedT(emailLocale(locale), "emails")("verify.subject")
}

export const VerifyEmail = ({ name, verifyUrl, locale }: VerifyEmailProps) => {
  const lng = emailLocale(locale)
  const t = i18n.getFixedT(lng, "emails")
  return (
    <Tailwind>
      <Html lang={lng}>
        <Head />

        <Body className="bg-bg-2 m-0 p-0">
          <Preview>{t("verify.preview")}</Preview>
          <Container className="mx-auto w-full max-w-[640px]">
            <Section className="bg-bg-3 px-0 pt-14 text-center">
              <Section className="px-6 pb-[72px]">
                <Section className="mb-8">
                  <Img
                    src="https://storage.yandexcloud.net/nvelope/nvelope.png"
                    alt={BRAND_NAME}
                    height={40}
                    className="mx-auto block"
                  />
                </Section>

                <Section className="mx-auto mb-8 max-w-[448px]">
                  <Text className="text-[40px] leading-[48px] font-sans text-fg m-0">
                    {t("verify.heading")}
                  </Text>
                  <Text className="font-14 text-fg-2 m-0 mt-6 font-sans">
                    {t("verify.greeting", { name })}
                    <br />
                    {t("verify.instruction")}
                  </Text>
                </Section>

                <Button
                  href={verifyUrl}
                  className="border-button-border font-15 inline-block rounded-[8px] border bg-white px-[20px] py-[12px] font-sans text-[#1F2222]"
                >
                  {t("verify.button")}
                </Button>

                <Section className="mx-auto mt-8 max-w-[448px]">
                  <Text className="font-13 text-fg-2 m-0 font-sans">
                    {t("verify.linkHint")}
                  </Text>
                  <Text className="font-13 m-0 mt-2 font-sans">
                    <Link href={verifyUrl} className="text-fg-2 break-all">
                      {verifyUrl}
                    </Link>
                  </Text>
                </Section>
              </Section>
            </Section>

            <Section className="px-6 py-20 text-center">
              <Section className="mx-auto max-w-[320px]">
                <Text className="font-13 text-fg-2 m-0 font-sans">
                  {t("verify.about")}
                </Text>

                <Text className="font-11 text-fg-2 m-0 mt-8 font-sans">
                  {t("verify.ignore")}
                </Text>
              </Section>
            </Section>
          </Container>
        </Body>
      </Html>
    </Tailwind>
  )
}

// PreviewProps feeds the `react-email` dev preview with sample inputs. The
// build-emails script ignores these and passes its own placeholder props.
VerifyEmail.PreviewProps = {
  name: "Ada Lovelace",
  verifyUrl: "https://nvelope.ru/verify-email?token=preview-token",
  locale: "en",
} satisfies VerifyEmailProps

export default VerifyEmail

import { Link, createFileRoute } from "@tanstack/react-router"
import { useEffect, useRef, useState } from "react"
import { useMutation } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { api } from "@/lib/api"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { FormField } from "@/components/common/form-field"

export const Route = createFileRoute("/verify-email")({
  component: VerifyEmail,
  validateSearch: (search: Record<string, unknown>): { token: string } => ({
    token: typeof search.token === "string" ? search.token : "",
  }),
})

export function VerifyEmail() {
  const { t } = useTranslation("auth")
  const { token } = Route.useSearch()
  const [resendEmail, setResendEmail] = useState("")

  // Verification is a state-changing POST, so it is a mutation — not a query.
  // It is fired exactly once from an effect, guarded by a ref, so it never
  // re-runs on a refetch trigger such as a window-focus.
  const verify = useMutation({
    mutationFn: () => api.verifyEmail(token),
  })
  const fired = useRef(false)
  useEffect(() => {
    if (token !== "" && !fired.current) {
      fired.current = true
      verify.mutate()
    }
  }, [token, verify.mutate])

  const resend = useMutation({
    mutationFn: (email: string) => api.resendVerification(email),
  })

  let title: string = t("verifyEmail.title")
  let description: string = t("verifyEmail.verifying")
  if (token === "") {
    title = t("verifyEmail.invalidTitle")
    description = t("verifyEmail.missingToken")
  } else if (verify.isError) {
    title = t("verifyEmail.invalidTitle")
    description = t("verifyEmail.invalidDescription")
  } else if (verify.isSuccess) {
    if (verify.data.data.verification.status === "already_verified") {
      title = t("verifyEmail.alreadyVerifiedTitle")
      description = t("verifyEmail.alreadyVerifiedDescription")
    } else {
      title = t("verifyEmail.verifiedTitle")
      description = t("verifyEmail.verifiedDescription")
    }
  }

  // A missing or invalid link is recoverable — offer a fresh one.
  const showResend = token === "" || verify.isError
  const done = token === "" || verify.isError || verify.isSuccess

  return (
    <main className="grid min-h-svh place-items-center p-6">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle>{title}</CardTitle>
          <CardDescription>{description}</CardDescription>
        </CardHeader>
        {showResend && (
          <CardContent>
            {resend.isSuccess ? (
              <p className="text-sm text-muted-foreground">
                {t("verifyEmail.resendSent")}
              </p>
            ) : (
              <form
                className="flex flex-col gap-3"
                noValidate
                onSubmit={(e) => {
                  e.preventDefault()
                  resend.mutate(resendEmail.trim())
                }}
              >
                <p className="text-sm text-muted-foreground">
                  {t("verifyEmail.resendPrompt")}
                </p>
                <FormField
                  label={t("verifyEmail.resendEmailLabel")}
                  type="email"
                  autoComplete="email"
                  value={resendEmail}
                  onChange={(e) => setResendEmail(e.target.value)}
                />
                <Button
                  type="submit"
                  disabled={resend.isPending || resendEmail.trim() === ""}
                >
                  {resend.isPending
                    ? t("verifyEmail.resendSending")
                    : t("verifyEmail.resendButton")}
                </Button>
              </form>
            )}
          </CardContent>
        )}
        {done && (
          <CardFooter>
            <Link
              className="text-sm text-primary underline-offset-4 hover:underline"
              to="/login"
            >
              {t("verifyEmail.goToLogin")}
            </Link>
          </CardFooter>
        )}
      </Card>
    </main>
  )
}

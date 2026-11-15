import { Link, createFileRoute, useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { useForm } from "@tanstack/react-form"
import { useMutation } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { api } from "@/lib/api"
import { queryClient, queryKeys } from "@/lib/query"
import { usePlatformConfig } from "@/hooks/use-platform-config"
import { ApiError, errorMessage, isConflict } from "@/lib/errors"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { FormField, compose, fieldError, rules } from "@/components/common/form-field"

export const Route = createFileRoute("/signup")({ component: Signup })

export function Signup() {
  const { t } = useTranslation("auth")
  const navigate = useNavigate()
  const { setupRequired } = usePlatformConfig()
  const [formError, setFormError] = useState("")
  const [emailTaken, setEmailTaken] = useState(false)
  const [sentTo, setSentTo] = useState("")

  const signup = useMutation({
    mutationFn: (v: { name: string; email: string; password: string; setupToken: string }) =>
      setupRequired
        ? api.signup(v.email.trim(), v.password, v.name.trim(), v.setupToken.trim())
        : api.signup(v.email.trim(), v.password, v.name.trim()),
    onSuccess: async (res) => {
      if ("tenant" in res.data) {
        // First-run setup signs the owner in and creates the workspace.
        await queryClient.invalidateQueries({ queryKey: queryKeys.me() })
        await queryClient.invalidateQueries({ queryKey: queryKeys.platformConfig() })
        await navigate({ to: "/t/$slug", params: { slug: res.data.tenant.slug } })
        return
      }
      // No session is issued — the account must verify its email first.
      setSentTo(res.data.verification.email)
    },
    onError: async (e) => {
      if (setupRequired && e instanceof ApiError && e.slug === "slug_taken") {
        // Lost the setup race after the owner was signed in: go to the landing page.
        await queryClient.invalidateQueries({ queryKey: queryKeys.me() })
        await navigate({ to: "/" })
        return
      }
      if (isConflict(e)) {
        setEmailTaken(true)
        return
      }
      if (e instanceof ApiError && e.slug === "email_domain_not_allowed") {
        setFormError(t("signup.emailDomainNotAllowed"))
        return
      }
      if (e instanceof ApiError && e.slug === "setup_token_invalid") {
        setFormError(t("signup.setupTokenInvalid"))
        return
      }
      if (e instanceof ApiError && e.slug === "setup_complete") {
        setFormError(t("signup.setupComplete"))
        return
      }
      setFormError(errorMessage(e))
    },
  })

  const form = useForm({
    defaultValues: { name: "", email: "", password: "", setupToken: "" },
    onSubmit: async ({ value }) => {
      setFormError("")
      setEmailTaken(false)
      await signup.mutateAsync(value).catch(() => {})
    },
  })

  if (sentTo) {
    return (
      <main className="grid min-h-svh place-items-center p-6">
        <Card className="w-full max-w-sm">
          <CardHeader>
            <CardTitle>{t("signup.checkInboxTitle")}</CardTitle>
            <CardDescription>
              {t("signup.checkInboxDescription", { email: sentTo })}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <p className="text-sm text-muted-foreground">
              {t("signup.checkInboxHint")}
            </p>
          </CardContent>
          <CardFooter>
            <Link
              className="text-sm text-primary underline-offset-4 hover:underline"
              to="/login"
            >
              {t("signup.backToLogin")}
            </Link>
          </CardFooter>
        </Card>
      </main>
    )
  }

  return (
    <main className="grid min-h-svh place-items-center p-6">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle>{t(setupRequired ? "signup.setupTitle" : "signup.title")}</CardTitle>
          <CardDescription>
            {t(setupRequired ? "signup.setupDescription" : "signup.description")}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form
            className="flex flex-col gap-4"
            noValidate
            onSubmit={(e) => {
              e.preventDefault()
              form.handleSubmit()
            }}
          >
            {formError && (
              <Alert variant="destructive">
                <AlertTitle>{t("signup.errorTitle")}</AlertTitle>
                <AlertDescription>{formError}</AlertDescription>
              </Alert>
            )}
            <form.Field
              name="name"
              validators={{
                onBlur: compose(rules.required(t("signup.nameRequired"))),
              }}
            >
              {(field) => (
                <FormField
                  label={t("signup.name")}
                  required
                  autoComplete="name"
                  value={field.state.value}
                  onBlur={field.handleBlur}
                  onChange={(e) => field.handleChange(e.target.value)}
                  error={fieldError(field.state.meta.errors)}
                />
              )}
            </form.Field>
            <form.Field
              name="email"
              validators={{ onBlur: compose(rules.required(), rules.email()) }}
            >
              {(field) => (
                <FormField
                  label={t("signup.email")}
                  type="email"
                  required
                  autoComplete="email"
                  value={field.state.value}
                  onBlur={field.handleBlur}
                  onChange={(e) => {
                    setEmailTaken(false)
                    field.handleChange(e.target.value)
                  }}
                  error={
                    emailTaken
                      ? t("signup.emailTaken")
                      : fieldError(field.state.meta.errors)
                  }
                />
              )}
            </form.Field>
            <form.Field
              name="password"
              validators={{
                onBlur: compose(
                  rules.minLength(8, t("signup.passwordTooShort")),
                ),
              }}
            >
              {(field) => (
                <FormField
                  label={t("signup.password")}
                  type="password"
                  required
                  autoComplete="new-password"
                  hint={t("signup.passwordHint")}
                  value={field.state.value}
                  onBlur={field.handleBlur}
                  onChange={(e) => field.handleChange(e.target.value)}
                  error={fieldError(field.state.meta.errors)}
                />
              )}
            </form.Field>
            {setupRequired && (
              <form.Field
                name="setupToken"
                validators={{
                  onBlur: compose(rules.required(t("signup.setupTokenRequired"))),
                }}
              >
                {(field) => (
                  <FormField
                    label={t("signup.setupToken")}
                    type="password"
                    required
                    autoComplete="one-time-code"
                    hint={t("signup.setupTokenHint")}
                    value={field.state.value}
                    onBlur={field.handleBlur}
                    onChange={(e) => field.handleChange(e.target.value)}
                    error={fieldError(field.state.meta.errors)}
                  />
                )}
              </form.Field>
            )}
            <Button type="submit" disabled={signup.isPending}>
              {signup.isPending ? t("signup.submitting") : t(setupRequired ? "signup.setupSubmit" : "signup.submit")}
            </Button>
          </form>
        </CardContent>
        <CardFooter>
          <p className="text-sm text-muted-foreground">
            {t("signup.haveAccount")}{" "}
            <Link
              className="text-primary underline-offset-4 hover:underline"
              to="/login"
            >
              {t("signup.logInLink")}
            </Link>
          </p>
        </CardFooter>
      </Card>
    </main>
  )
}

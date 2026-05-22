import { Link, createFileRoute, useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { useForm } from "@tanstack/react-form"
import { useMutation } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { api } from "@/lib/api"
import { errorMessage, isUnauthorized } from "@/lib/errors"
import { queryClient } from "@/lib/query"
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

export const Route = createFileRoute("/login")({ component: Login })

export function Login() {
  const navigate = useNavigate()
  const { t } = useTranslation("auth")
  const [formError, setFormError] = useState("")

  const login = useMutation({
    mutationFn: (v: { email: string; password: string }) =>
      api.login(v.email.trim(), v.password),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["me"] })
      await navigate({ to: "/" })
    },
    onError: (e) => {
      // Invalid credentials are reported non-specifically (FR-002) — never
      // reveal whether the email or the password was wrong.
      setFormError(
        isUnauthorized(e) ? t("login.invalidCredentials") : errorMessage(e),
      )
    },
  })

  const form = useForm({
    defaultValues: { email: "", password: "" },
    onSubmit: async ({ value }) => {
      setFormError("")
      await login.mutateAsync(value).catch(() => {})
    },
  })

  return (
    <main className="grid min-h-svh place-items-center p-6">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle>{t("login.title")}</CardTitle>
          <CardDescription>{t("login.description")}</CardDescription>
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
                <AlertTitle>{t("login.errorTitle")}</AlertTitle>
                <AlertDescription>{formError}</AlertDescription>
              </Alert>
            )}
            <form.Field
              name="email"
              validators={{ onBlur: compose(rules.required(), rules.email()) }}
            >
              {(field) => (
                <FormField
                  label={t("login.email")}
                  type="email"
                  required
                  autoComplete="email"
                  value={field.state.value}
                  onBlur={field.handleBlur}
                  onChange={(e) => field.handleChange(e.target.value)}
                  error={fieldError(field.state.meta.errors)}
                />
              )}
            </form.Field>
            <form.Field
              name="password"
              validators={{ onBlur: compose(rules.required()) }}
            >
              {(field) => (
                <FormField
                  label={t("login.password")}
                  type="password"
                  required
                  autoComplete="current-password"
                  value={field.state.value}
                  onBlur={field.handleBlur}
                  onChange={(e) => field.handleChange(e.target.value)}
                  error={fieldError(field.state.meta.errors)}
                />
              )}
            </form.Field>
            <Button type="submit" disabled={login.isPending}>
              {login.isPending ? t("login.submitting") : t("login.submit")}
            </Button>
          </form>
        </CardContent>
        <CardFooter>
          <p className="text-sm text-muted-foreground">
            {t("login.needAccount")}{" "}
            <Link
              className="text-primary underline-offset-4 hover:underline"
              to="/signup"
            >
              {t("login.signUpLink")}
            </Link>
          </p>
        </CardFooter>
      </Card>
    </main>
  )
}

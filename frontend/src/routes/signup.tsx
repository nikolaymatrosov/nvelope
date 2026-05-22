import { Link, createFileRoute, useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { useForm } from "@tanstack/react-form"
import { useMutation } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { api } from "@/lib/api"
import { errorMessage, isConflict } from "@/lib/errors"
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

export const Route = createFileRoute("/signup")({ component: Signup })

export function Signup() {
  const navigate = useNavigate()
  const { t } = useTranslation("auth")
  const [formError, setFormError] = useState("")
  const [emailTaken, setEmailTaken] = useState(false)

  const signup = useMutation({
    mutationFn: (v: { name: string; email: string; password: string }) =>
      api.signup(v.email.trim(), v.password, v.name.trim()),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["me"] })
      await navigate({ to: "/tenants/new" })
    },
    onError: (e) => {
      if (isConflict(e)) {
        setEmailTaken(true)
        return
      }
      setFormError(errorMessage(e))
    },
  })

  const form = useForm({
    defaultValues: { name: "", email: "", password: "" },
    onSubmit: async ({ value }) => {
      setFormError("")
      setEmailTaken(false)
      await signup.mutateAsync(value).catch(() => {})
    },
  })

  return (
    <main className="grid min-h-svh place-items-center p-6">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle>{t("signup.title")}</CardTitle>
          <CardDescription>{t("signup.description")}</CardDescription>
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
            <Button type="submit" disabled={signup.isPending}>
              {signup.isPending ? t("signup.submitting") : t("signup.submit")}
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

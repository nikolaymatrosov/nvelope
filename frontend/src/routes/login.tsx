import { Link, createFileRoute, useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { useForm } from "@tanstack/react-form"
import { useMutation } from "@tanstack/react-query"
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
        isUnauthorized(e)
          ? "That email and password do not match."
          : errorMessage(e),
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
          <CardTitle>Log in to nvelope</CardTitle>
          <CardDescription>
            Welcome back. Sign in to your workspaces.
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
                <AlertTitle>Could not sign in</AlertTitle>
                <AlertDescription>{formError}</AlertDescription>
              </Alert>
            )}
            <form.Field
              name="email"
              validators={{ onBlur: compose(rules.required(), rules.email()) }}
            >
              {(field) => (
                <FormField
                  label="Email"
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
                  label="Password"
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
              {login.isPending ? "Signing in…" : "Log in"}
            </Button>
          </form>
        </CardContent>
        <CardFooter>
          <p className="text-sm text-muted-foreground">
            Need an account?{" "}
            <Link
              className="text-primary underline-offset-4 hover:underline"
              to="/signup"
            >
              Sign up
            </Link>
          </p>
        </CardFooter>
      </Card>
    </main>
  )
}

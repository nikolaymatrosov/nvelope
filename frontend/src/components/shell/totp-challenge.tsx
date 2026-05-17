// Session TOTP challenge (FR-027, research.md Decision 5). Shown when opening a
// workspace session returns `totp_pending`; on a verified code the workspace
// shell is revealed.

import { useState } from "react"
import { useForm } from "@tanstack/react-form"
import { useMutation } from "@tanstack/react-query"
import { api } from "@/lib/api"
import { errorMessage } from "@/lib/errors"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { FormField, compose, fieldError, rules } from "@/components/common/form-field"

type TotpChallengeProps = {
  slug: string
  onVerified: () => void
}

export function TotpChallenge({ slug, onVerified }: TotpChallengeProps) {
  const [serverError, setServerError] = useState("")

  const verify = useMutation({
    mutationFn: (code: string) => api.verifySessionTOTP(slug, code.trim()),
    onSuccess: (res) => {
      if (res.data.state === "active") {
        onVerified()
      } else {
        setServerError(
          "That code did not work. Try the current code from your app.",
        )
      }
    },
    onError: (e) => setServerError(errorMessage(e)),
  })

  const form = useForm({
    defaultValues: { code: "" },
    onSubmit: async ({ value, formApi }) => {
      setServerError("")
      try {
        await verify.mutateAsync(value.code)
        formApi.reset()
      } catch {
        formApi.setFieldValue("code", "")
      }
    },
  })

  return (
    <main className="grid min-h-svh place-items-center p-6">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle>Two-factor verification</CardTitle>
          <CardDescription>
            Enter the 6-digit code from your authenticator app to continue.
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
            <form.Field
              name="code"
              validators={{
                onSubmit: compose(rules.required("Enter the 6-digit code.")),
              }}
            >
              {(field) => (
                <FormField
                  label="Authentication code"
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  autoFocus
                  placeholder="123456"
                  value={field.state.value}
                  onChange={(e) => {
                    setServerError("")
                    field.handleChange(e.target.value)
                  }}
                  error={serverError || fieldError(field.state.meta.errors)}
                />
              )}
            </form.Field>
            <Button type="submit" disabled={verify.isPending}>
              {verify.isPending ? "Verifying…" : "Verify"}
            </Button>
          </form>
        </CardContent>
      </Card>
    </main>
  )
}

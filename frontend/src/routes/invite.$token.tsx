import { createFileRoute, useNavigate } from "@tanstack/react-router"
import { useEffect, useState } from "react"
import type { FormEvent } from "react"
import { Button } from "@/components/ui/button"
import { api } from "@/lib/api"

export const Route = createFileRoute("/invite/$token")({ component: AcceptInvite })

type State =
  | { phase: "loading" }
  | { phase: "invalid" }
  | { phase: "ready"; tenantName: string; tenantSlug: string; email: string }

function AcceptInvite() {
  const { token } = Route.useParams()
  const navigate = useNavigate()
  const [state, setState] = useState<State>({ phase: "loading" })
  const [password, setPassword] = useState("")
  const [name, setName] = useState("")
  const [error, setError] = useState("")
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    api.getInvitation(token).then(({ status, data }) => {
      if (status === 200) {
        setState({
          phase: "ready",
          tenantName: data.tenant.name,
          tenantSlug: data.tenant.slug,
          email: data.email,
        })
      } else {
        setState({ phase: "invalid" })
      }
    })
  }, [token])

  async function accept(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError("")
    const { status, data } = await api.acceptInvitation(token, password, name)
    setBusy(false)
    if (status === 200) {
      navigate({ to: "/t/$slug", params: { slug: data.tenant.slug } })
      return
    }
    setError(data?.message ?? "Could not accept the invitation.")
  }

  if (state.phase === "loading") {
    return <main className="p-6 text-sm">Loading…</main>
  }
  if (state.phase === "invalid") {
    return (
      <main className="mx-auto flex max-w-sm flex-col gap-3 p-6">
        <h1 className="text-xl font-semibold">Invitation not valid</h1>
        <p className="text-sm">
          This invitation has expired, been revoked, or already been used.
        </p>
      </main>
    )
  }

  return (
    <main className="mx-auto flex min-h-svh max-w-sm flex-col justify-center gap-4 p-6">
      <h1 className="text-xl font-semibold">Join {state.tenantName}</h1>
      <p className="text-sm">
        You were invited as <strong>{state.email}</strong>. Create your account
        to join.
      </p>
      <form className="flex flex-col gap-3" onSubmit={accept}>
        <input
          className="rounded border px-3 py-2"
          placeholder="Your name"
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
        <input
          className="rounded border px-3 py-2"
          type="password"
          placeholder="Choose a password (at least 8 characters)"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
        {error && <p className="text-sm text-red-600">{error}</p>}
        <Button type="submit" disabled={busy}>
          {busy ? "Joining…" : "Accept invitation"}
        </Button>
      </form>
      <p className="text-xs text-gray-500">
        Already have an account? Log in first, then open this link again.
      </p>
    </main>
  )
}

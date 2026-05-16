import { Link, createFileRoute, useNavigate } from "@tanstack/react-router"
import { useEffect, useState } from "react"
import { Button } from "@/components/ui/button"
import { api } from "@/lib/api"

export const Route = createFileRoute("/")({ component: Home })

type Membership = { id: string; slug: string; name: string; role: string }
type State =
  | { phase: "loading" }
  | { phase: "anonymous" }
  | { phase: "ready"; name: string; tenants: Array<Membership> }

function Home() {
  const navigate = useNavigate()
  const [state, setState] = useState<State>({ phase: "loading" })

  useEffect(() => {
    api.me().then(({ status, data }) => {
      if (status === 200) {
        setState({ phase: "ready", name: data.user.name, tenants: data.tenants })
      } else {
        setState({ phase: "anonymous" })
      }
    })
  }, [])

  async function logout() {
    await api.logout()
    setState({ phase: "anonymous" })
  }

  if (state.phase === "loading") {
    return <main className="p-6 text-sm">Loading…</main>
  }

  if (state.phase === "anonymous") {
    return (
      <main className="mx-auto flex min-h-svh max-w-sm flex-col justify-center gap-4 p-6">
        <h1 className="text-xl font-semibold">nvelope</h1>
        <p className="text-sm">Sign in to manage your workspaces.</p>
        <div className="flex gap-3">
          <Button onClick={() => navigate({ to: "/login" })}>Log in</Button>
          <Button variant="outline" onClick={() => navigate({ to: "/signup" })}>
            Sign up
          </Button>
        </div>
      </main>
    )
  }

  return (
    <main className="mx-auto flex max-w-md flex-col gap-4 p-6">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold">Your workspaces</h1>
        <Button variant="outline" onClick={logout}>
          Log out
        </Button>
      </div>
      <p className="text-sm">Signed in as {state.name}.</p>
      {state.tenants.length === 0 ? (
        <p className="text-sm">You don't belong to any workspace yet.</p>
      ) : (
        <ul className="flex flex-col gap-2">
          {state.tenants.map((t) => (
            <li key={t.id}>
              <Link
                className="underline"
                to="/t/$slug"
                params={{ slug: t.slug }}
              >
                {t.name}
              </Link>{" "}
              <span className="text-xs text-gray-500">({t.role})</span>
            </li>
          ))}
        </ul>
      )}
      <Button onClick={() => navigate({ to: "/tenants/new" })}>
        Create a workspace
      </Button>
    </main>
  )
}

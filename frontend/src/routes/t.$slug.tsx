import { Link, createFileRoute } from "@tanstack/react-router"
import { useCallback, useEffect, useState } from "react"
import type { FormEvent } from "react"
import { Button } from "@/components/ui/button"
import { api } from "@/lib/api"

export const Route = createFileRoute("/t/$slug")({ component: Workspace })

type Member = { user_id: string; email: string; name: string; role: string }
type Invitation = { id: string; email: string; expires_at: string }
type State =
  | { phase: "loading" }
  | { phase: "notfound" }
  | { phase: "ready"; name: string; members: Array<Member>; invitations: Array<Invitation> }

function Workspace() {
  const { slug } = Route.useParams()
  const [state, setState] = useState<State>({ phase: "loading" })
  const [inviteEmail, setInviteEmail] = useState("")
  const [inviteMsg, setInviteMsg] = useState("")
  const [acceptUrl, setAcceptUrl] = useState("")

  const load = useCallback(async () => {
    const [tenant, invites] = await Promise.all([
      api.tenant(slug),
      api.listInvitations(slug),
    ])
    if (tenant.status !== 200) {
      setState({ phase: "notfound" })
      return
    }
    setState({
      phase: "ready",
      name: tenant.data.tenant.name,
      members: tenant.data.members,
      invitations: invites.status === 200 ? invites.data.invitations : [],
    })
  }, [slug])

  useEffect(() => {
    load()
  }, [load])

  async function sendInvite(e: FormEvent) {
    e.preventDefault()
    setInviteMsg("")
    setAcceptUrl("")
    const { status, data } = await api.invite(slug, inviteEmail)
    if (status === 201) {
      setInviteEmail("")
      setAcceptUrl(data.accept_url)
      await load()
      return
    }
    setInviteMsg(data?.message ?? "Could not send the invitation.")
  }

  if (state.phase === "loading") {
    return <main className="p-6 text-sm">Loading…</main>
  }
  if (state.phase === "notfound") {
    return (
      <main className="mx-auto flex max-w-md flex-col gap-3 p-6">
        <h1 className="text-xl font-semibold">Workspace not found</h1>
        <p className="text-sm">
          This workspace does not exist, or you are not a member.
        </p>
        <Link className="underline" to="/">
          Back to your workspaces
        </Link>
      </main>
    )
  }

  return (
    <main className="mx-auto flex max-w-md flex-col gap-5 p-6">
      <div>
        <Link className="text-sm underline" to="/">
          ← Workspaces
        </Link>
        <h1 className="mt-1 text-xl font-semibold">{state.name}</h1>
      </div>

      <section>
        <h2 className="text-sm font-medium">Members</h2>
        <ul className="mt-1 flex flex-col gap-1 text-sm">
          {state.members.map((m) => (
            <li key={m.user_id}>
              {m.name} — {m.email}{" "}
              <span className="text-xs text-gray-500">({m.role})</span>
            </li>
          ))}
        </ul>
      </section>

      <section className="flex flex-col gap-2">
        <h2 className="text-sm font-medium">Invite a teammate</h2>
        <form className="flex gap-2" onSubmit={sendInvite}>
          <input
            className="flex-1 rounded border px-3 py-2 text-sm"
            type="email"
            placeholder="teammate@example.com"
            value={inviteEmail}
            onChange={(e) => setInviteEmail(e.target.value)}
          />
          <Button type="submit">Invite</Button>
        </form>
        {inviteMsg && <p className="text-sm text-red-600">{inviteMsg}</p>}
        {acceptUrl && (
          <p className="text-sm">
            Share this link with your teammate:{" "}
            <code className="break-all">{acceptUrl}</code>
          </p>
        )}
        {state.invitations.length > 0 && (
          <ul className="flex flex-col gap-1 text-sm">
            {state.invitations.map((inv) => (
              <li key={inv.id} className="text-gray-600">
                Pending: {inv.email}
              </li>
            ))}
          </ul>
        )}
      </section>
    </main>
  )
}

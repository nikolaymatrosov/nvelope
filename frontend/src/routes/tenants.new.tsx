import { createFileRoute, useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import type { FormEvent } from "react"
import { Button } from "@/components/ui/button"
import { api } from "@/lib/api"

export const Route = createFileRoute("/tenants/new")({ component: NewTenant })

function NewTenant() {
  const navigate = useNavigate()
  const [name, setName] = useState("")
  const [slug, setSlug] = useState("")
  const [error, setError] = useState("")
  const [busy, setBusy] = useState(false)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError("")
    const { status, data } = await api.createTenant(name, slug)
    setBusy(false)
    if (status === 201) {
      navigate({ to: "/t/$slug", params: { slug: data.tenant.slug } })
      return
    }
    setError(data?.message ?? "Could not create the workspace.")
  }

  return (
    <main className="mx-auto flex min-h-svh max-w-sm flex-col justify-center gap-4 p-6">
      <h1 className="text-xl font-semibold">Create a workspace</h1>
      <form className="flex flex-col gap-3" onSubmit={submit}>
        <input
          className="rounded border px-3 py-2"
          placeholder="Workspace name"
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
        <input
          className="rounded border px-3 py-2"
          placeholder="Workspace address (optional — derived from the name)"
          value={slug}
          onChange={(e) => setSlug(e.target.value)}
        />
        {error && <p className="text-sm text-red-600">{error}</p>}
        <Button type="submit" disabled={busy}>
          {busy ? "Creating…" : "Create workspace"}
        </Button>
      </form>
    </main>
  )
}

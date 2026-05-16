import { Link, createFileRoute, useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import type { FormEvent } from "react"
import { Button } from "@/components/ui/button"
import { api } from "@/lib/api"

export const Route = createFileRoute("/login")({ component: Login })

function Login() {
  const navigate = useNavigate()
  const [email, setEmail] = useState("")
  const [password, setPassword] = useState("")
  const [error, setError] = useState("")
  const [busy, setBusy] = useState(false)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError("")
    const { status, data } = await api.login(email, password)
    setBusy(false)
    if (status === 200) {
      navigate({ to: "/" })
      return
    }
    setError(data?.message ?? "Log-in failed.")
  }

  return (
    <main className="mx-auto flex min-h-svh max-w-sm flex-col justify-center gap-4 p-6">
      <h1 className="text-xl font-semibold">Log in to nvelope</h1>
      <form className="flex flex-col gap-3" onSubmit={submit}>
        <input
          className="rounded border px-3 py-2"
          type="email"
          placeholder="Email"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
        />
        <input
          className="rounded border px-3 py-2"
          type="password"
          placeholder="Password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
        {error && <p className="text-sm text-red-600">{error}</p>}
        <Button type="submit" disabled={busy}>
          {busy ? "Logging in…" : "Log in"}
        </Button>
      </form>
      <p className="text-sm">
        Need an account?{" "}
        <Link className="underline" to="/signup">
          Sign up
        </Link>
      </p>
    </main>
  )
}

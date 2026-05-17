import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { TotpPanel } from "./index"
import { renderWithClient } from "@/test/render"

import { api } from "@/lib/api"
import { TotpChallenge } from "@/components/shell/totp-challenge"

vi.mock("@tanstack/react-router", () => ({
  createFileRoute: () => (opts: object) => opts,
}))

vi.mock("@/lib/api", () => ({
  api: {
    enableTOTP: vi.fn(),
    confirmTOTP: vi.fn(),
    disableTOTP: vi.fn(),
    verifySessionTOTP: vi.fn(),
    listAPIKeys: vi.fn(),
  },
}))

const ok = <T,>(data: T) => ({ status: 200, ok: true, data })

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("TotpPanel", () => {
  it("enrols and confirms a TOTP secret", async () => {
    vi.mocked(api.enableTOTP).mockResolvedValue(
      ok({ secret: "JBSWY3DPEHPK3PXP", uri: "otpauth://totp/x" }),
    )
    vi.mocked(api.confirmTOTP).mockResolvedValue(
      ok({ recovery_codes: ["aaaa-bbbb", "cccc-dddd"] }),
    )
    renderWithClient(<TotpPanel slug="acme" />)

    fireEvent.click(screen.getByRole("button", { name: /enable two-factor/i }))
    expect(await screen.findByText("JBSWY3DPEHPK3PXP")).toBeDefined()

    fireEvent.change(screen.getByLabelText(/authentication code/i), {
      target: { value: "123456" },
    })
    fireEvent.click(screen.getByRole("button", { name: /confirm/i }))

    await waitFor(() =>
      expect(api.confirmTOTP).toHaveBeenCalledWith(
        "acme",
        "JBSWY3DPEHPK3PXP",
        "123456",
      ),
    )
    expect(await screen.findByText("aaaa-bbbb")).toBeDefined()
  })
})

describe("TotpChallenge", () => {
  it("verifies a code and signals success", async () => {
    vi.mocked(api.verifySessionTOTP).mockResolvedValue(ok({ state: "active" }))
    const onVerified = vi.fn()
    renderWithClient(<TotpChallenge slug="acme" onVerified={onVerified} />)

    fireEvent.change(screen.getByLabelText(/authentication code/i), {
      target: { value: "654321" },
    })
    fireEvent.click(screen.getByRole("button", { name: /verify/i }))

    await waitFor(() =>
      expect(api.verifySessionTOTP).toHaveBeenCalledWith("acme", "654321"),
    )
    await waitFor(() => expect(onVerified).toHaveBeenCalled())
  })

  it("shows an error on an invalid code", async () => {
    vi.mocked(api.verifySessionTOTP).mockResolvedValue(
      ok({ state: "totp_pending" }),
    )
    renderWithClient(<TotpChallenge slug="acme" onVerified={vi.fn()} />)

    fireEvent.change(screen.getByLabelText(/authentication code/i), {
      target: { value: "000000" },
    })
    fireEvent.click(screen.getByRole("button", { name: /verify/i }))

    expect(await screen.findByText(/that code did not work/i)).toBeDefined()
  })
})

import { DnsRecordRow } from "./dns-record-row"
import type { Meta, StoryObj } from "@storybook/react-vite"

const meta = {
  component: DnsRecordRow,
  tags: ["ai-generated"],
  args: {
    recordType: "DKIM",
    host: "mail._domainkey.example.com",
    value: "v=DKIM1; k=rsa; p=MIGfMA0GCSqGSIb3DQEBAQUAA4GN...",
  },
} satisfies Meta<typeof DnsRecordRow>

export default meta
type Story = StoryObj<typeof meta>

// A DKIM record with its long public-key value.
export const Dkim: Story = {}

// SPF records carry the sending policy as their value.
export const Spf: Story = {
  args: {
    recordType: "SPF",
    host: "example.com",
    value: "v=spf1 include:_spf.example-esp.com ~all",
  },
}

// DMARC alignment/reporting policy.
export const Dmarc: Story = {
  args: {
    recordType: "DMARC",
    host: "_dmarc.example.com",
    value: "v=DMARC1; p=quarantine; rua=mailto:dmarc@example.com",
  },
}

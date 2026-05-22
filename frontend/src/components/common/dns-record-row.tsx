// A copyable DNS record row (DKIM / SPF / DMARC) for the sending-domain
// detail view (FR-005). Each record field is individually copyable.

import { useState } from "react"
import { CheckIcon, CopyIcon } from "lucide-react"
import { toast } from "sonner"
import { useTranslation } from "react-i18next"
import { Button } from "@/components/ui/button"

function CopyButton({ value, label }: { value: string; label: string }) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)

  async function copy() {
    try {
      await navigator.clipboard.writeText(value)
      setCopied(true)
      toast.success(t("dnsRecordRow.copied", { label }))
      setTimeout(() => setCopied(false), 1500)
    } catch {
      toast.error(t("clipboard.copyToClipboardFailed"))
    }
  }

  return (
    <Button
      type="button"
      variant="ghost"
      size="icon-sm"
      aria-label={t("dnsRecordRow.copyLabel", { label })}
      onClick={copy}
    >
      {copied ? <CheckIcon className="text-primary" /> : <CopyIcon />}
    </Button>
  )
}

type DnsRecordRowProps = {
  recordType: string
  host: string
  value: string
}

export function DnsRecordRow({ recordType, host, value }: DnsRecordRowProps) {
  const { t } = useTranslation()
  return (
    <div className="flex flex-col gap-2 rounded-lg border p-3">
      <div className="flex items-center justify-between">
        <span className="text-xs font-semibold tracking-wide text-muted-foreground uppercase">
          {recordType}
        </span>
      </div>
      <div className="grid grid-cols-[auto_1fr_auto] items-center gap-2">
        <span className="text-xs text-muted-foreground">
          {t("dnsRecordRow.host")}
        </span>
        <code className="truncate rounded bg-muted px-2 py-1 font-mono text-xs">
          {host}
        </code>
        <CopyButton
          value={host}
          label={t("dnsRecordRow.hostLabel", { recordType })}
        />
        <span className="text-xs text-muted-foreground">
          {t("dnsRecordRow.value")}
        </span>
        <code className="truncate rounded bg-muted px-2 py-1 font-mono text-xs">
          {value}
        </code>
        <CopyButton
          value={value}
          label={t("dnsRecordRow.valueLabel", { recordType })}
        />
      </div>
    </div>
  )
}

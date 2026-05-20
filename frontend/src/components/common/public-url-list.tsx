// One place where an administrator can copy the per-tenant public URLs they
// might share — subscription pages, the preference-link template, the public
// archive index, and the RSS feed (FR-030). The token-template row carries an
// explanatory hint that the token is filled per subscriber.

import { CopyIcon, ExternalLinkIcon } from "lucide-react"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"

export type PublicUrlKind = "subscription" | "preference-template" | "archive" | "rss"

export type PublicUrlRow = {
  label: string
  url: string
  kind: PublicUrlKind
  // When set, the row links to a separate "preview" URL — used for the
  // preference-link template, where the user-visible template URL is the
  // token-form template but the preview opens an example.
  previewUrl?: string
}

type PublicUrlListProps = {
  rows: Array<PublicUrlRow>
}

async function copy(url: string) {
  try {
    await navigator.clipboard.writeText(url)
    toast.success("Copied")
  } catch {
    toast.error("Could not copy — copy the URL manually.")
  }
}

function kindHint(kind: PublicUrlKind): string | undefined {
  if (kind === "preference-template") {
    return "The {token} placeholder is filled in automatically when this link is sent to a subscriber."
  }
  return undefined
}

export function PublicUrlList({ rows }: PublicUrlListProps) {
  if (rows.length === 0) {
    return (
      <p
        className="text-sm text-muted-foreground"
        data-testid="public-url-list-empty"
      >
        No public URLs to share yet.
      </p>
    )
  }
  return (
    <ul
      className="flex flex-col gap-2"
      data-testid="public-url-list"
    >
      {rows.map((row) => {
        const hint = kindHint(row.kind)
        const previewable = row.kind !== "preference-template"
        return (
          <li
            key={`${row.kind}:${row.url}`}
            className="flex flex-col gap-1 rounded-md border p-3"
            data-testid={`public-url-row-${row.kind}`}
          >
            <div className="flex items-center justify-between gap-2">
              <div className="min-w-0 flex-1">
                <p className="text-sm font-medium">{row.label}</p>
                <p className="truncate text-xs text-muted-foreground" title={row.url}>
                  {row.url}
                </p>
              </div>
              <div className="flex shrink-0 items-center gap-1">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => copy(row.url)}
                  aria-label={`Copy ${row.label}`}
                >
                  <CopyIcon /> Copy
                </Button>
                {previewable && (
                  <Button
                    variant="ghost"
                    size="sm"
                    asChild
                    aria-label={`Preview ${row.label}`}
                  >
                    <a
                      href={row.previewUrl ?? row.url}
                      target="_blank"
                      rel="noreferrer"
                    >
                      <ExternalLinkIcon /> Preview
                    </a>
                  </Button>
                )}
              </div>
            </div>
            {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
          </li>
        )
      })}
    </ul>
  )
}

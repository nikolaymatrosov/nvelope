// BlockLibrary — the slide-out «Block library» opened by «Add block» or the
// empty-canvas button. Left: block types under the «Basic» group (phase 2 adds
// «Sections», phase 4 «My sections»). Right: variant cards for the active type
// with tiny static previews. Clicking a card hands the catalog item to the
// caller, which inserts it (see blockCatalog.insertBlock).

import { useState } from "react"
import { useTranslation } from "react-i18next"
import { BLOCK_CATALOG, CATALOG_TYPES } from "../blockCatalog"
import type { CatalogItem, CatalogKey, CatalogType } from "../blockCatalog"
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet"

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onInsert: (item: CatalogItem) => void
}

export function BlockLibrary({ open, onOpenChange, onInsert }: Props) {
  const { t } = useTranslation("visualEditor")
  const [active, setActive] = useState<CatalogType>("text")
  const items = BLOCK_CATALOG.filter((i) => i.type === active)

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="left"
        className="w-[680px] max-w-[95vw] gap-0 p-0 sm:max-w-[680px]"
        data-testid="ve-library"
        aria-describedby={undefined}
      >
        <SheetHeader>
          <SheetTitle>{t("library.title")}</SheetTitle>
        </SheetHeader>
        <div className="ve-library__body">
          <nav className="ve-library__nav">
            <p className="ve-library__group">{t("library.group.basic")}</p>
            {CATALOG_TYPES.map((type) => (
              <button
                key={type}
                type="button"
                className={`ve-library__type${type === active ? " is-active" : ""}`}
                aria-current={type === active}
                data-testid={`ve-library-type-${type}`}
                onClick={() => setActive(type)}
              >
                {t(`catalog.type.${type}`)}
              </button>
            ))}
          </nav>
          <div className="ve-library__cards">
            {items.map((item) => (
              <button
                key={item.key}
                type="button"
                className="ve-library__card"
                data-testid={`ve-library-item-${item.key}`}
                onClick={() => onInsert(item)}
              >
                <CardPreview itemKey={item.key} />
                <span className="ve-library__card-label">{t(`catalog.item.${item.key}`)}</span>
              </button>
            ))}
          </div>
        </div>
      </SheetContent>
    </Sheet>
  )
}

// CardPreview draws a tiny static mock of the block. No image assets to keep.
function CardPreview({ itemKey }: { itemKey: CatalogKey }) {
  switch (itemKey) {
    case "heading-1":
      // i18next-instrument-ignore-next-line -- decorative specimen glyph, not copy
      return <span className="ve-mock ve-mock--h1">Aa</span>
    case "heading-2":
      // i18next-instrument-ignore-next-line -- decorative specimen glyph, not copy
      return <span className="ve-mock ve-mock--h2">Aa</span>
    case "heading-3":
      // i18next-instrument-ignore-next-line -- decorative specimen glyph, not copy
      return <span className="ve-mock ve-mock--h3">Aa</span>
    case "columns-2":
    case "columns-3":
    case "columns-4":
      return (
        <span className="ve-mock ve-mock--columns">
          {Array.from({ length: Number(itemKey.slice(-1)) }, (_, i) => (
            <span key={i} />
          ))}
        </span>
      )
    case "button":
      return <span className="ve-mock ve-mock--button" />
    case "image":
      return <span className="ve-mock ve-mock--image" />
    case "divider":
      return <span className="ve-mock ve-mock--divider" />
    case "raw-html":
      // i18next-instrument-ignore-next-line -- decorative specimen glyph, not copy
      return <span className="ve-mock ve-mock--code">{"</>"}</span>
    case "code":
      // i18next-instrument-ignore-next-line -- decorative specimen glyph, not copy
      return <span className="ve-mock ve-mock--code">{"{ }"}</span>
    case "bullet-list":
    case "ordered-list":
      return (
        <span className="ve-mock ve-mock--list">
          <span />
          <span />
          <span />
        </span>
      )
    case "quote":
      return (
        <span className="ve-mock ve-mock--quote">
          <span />
          <span />
        </span>
      )
    case "paragraph":
      return (
        <span className="ve-mock ve-mock--lines">
          <span />
          <span />
          <span />
        </span>
      )
  }
}

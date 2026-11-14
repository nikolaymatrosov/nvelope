// EditorTopBar — the full-screen editor's top bar: back + breadcrumbs, «Add
// block», undo/redo, desktop/mobile canvas width, Preview, and Save. Read-only
// sessions show a badge instead of the editing controls.

import { useTranslation } from "react-i18next"
import { ArrowLeft, Eye, Monitor, PanelRight, Plus, Redo2, Smartphone, Undo2 } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"

export type Device = "desktop" | "mobile"

type Props = {
  section: string
  name: string
  onBack: () => void
  editable: boolean
  device: Device
  onDeviceChange: (device: Device) => void
  canUndo: boolean
  canRedo: boolean
  onUndo: () => void
  onRedo: () => void
  onAdd: () => void
  onPreview: () => void
  dirty: boolean
  saving: boolean
  onSave: () => void
  // Present only on narrow viewports, where the right panel is a sheet.
  onOpenPanel?: () => void
}

const DEVICES: ReadonlyArray<Device> = ["desktop", "mobile"]

export function EditorTopBar(p: Props) {
  const { t } = useTranslation("visualEditor")
  const saveLabel = p.saving ? t("screen.saving") : p.dirty ? t("screen.save") : t("screen.saved")
  return (
    <header className="ve-topbar" data-testid="ve-topbar">
      <div className="ve-topbar__group">
        <Button type="button" variant="ghost" size="icon-sm" onClick={p.onBack} aria-label={t("screen.back")} data-testid="ve-back">
          <ArrowLeft />
        </Button>
        <nav className="ve-topbar__crumbs" aria-label={t("screen.back")}>
          <span>{p.section}</span>
          <span aria-hidden>/</span>
          <span className="ve-topbar__name" data-testid="ve-topbar-name">
            {p.name}
          </span>
        </nav>
        {p.editable ? (
          <>
            <Button type="button" size="sm" onClick={p.onAdd} data-testid="ve-add">
              <Plus /> {t("screen.add")}
            </Button>
            <Button type="button" variant="ghost" size="icon-sm" onClick={p.onUndo} disabled={!p.canUndo} aria-label={t("screen.undo")} data-testid="ve-undo">
              <Undo2 />
            </Button>
            <Button type="button" variant="ghost" size="icon-sm" onClick={p.onRedo} disabled={!p.canRedo} aria-label={t("screen.redo")} data-testid="ve-redo">
              <Redo2 />
            </Button>
          </>
        ) : (
          <Badge variant="secondary" data-testid="ve-readonly-badge">
            {t("screen.readOnly")}
          </Badge>
        )}
      </div>
      <div className="ve-topbar__group" role="group" aria-label={t("screen.device.label")}>
        {DEVICES.map((d) => (
          <Button
            key={d}
            type="button"
            variant={p.device === d ? "secondary" : "ghost"}
            size="icon-sm"
            aria-pressed={p.device === d}
            aria-label={t(`screen.device.${d}`)}
            data-testid={`ve-device-${d}`}
            onClick={() => p.onDeviceChange(d)}
          >
            {d === "desktop" ? <Monitor /> : <Smartphone />}
          </Button>
        ))}
        <Button type="button" variant="outline" size="sm" onClick={p.onPreview} data-testid="ve-preview">
          <Eye /> {t("screen.preview")}
        </Button>
      </div>
      <div className="ve-topbar__group">
        {p.onOpenPanel && (
          <Button type="button" variant="outline" size="sm" onClick={p.onOpenPanel} data-testid="ve-open-panel">
            <PanelRight /> {t("screen.openPanel")}
          </Button>
        )}
        {p.editable && (
          <Button type="button" size="sm" onClick={p.onSave} disabled={!p.dirty || p.saving} data-testid="ve-save">
            {saveLabel}
          </Button>
        )}
      </div>
    </header>
  )
}

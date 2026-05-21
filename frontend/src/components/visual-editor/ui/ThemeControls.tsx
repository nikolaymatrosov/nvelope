// ThemeControls — the per-row theme override panel that lives in the
// VisualEmailEditor chrome (T108 / US3 acceptance scenario 2).
//
// Behavior:
//   - When `value` is null, the row inherits tenant Phase 6 branding. The
//     panel surfaces a clear "Using tenant defaults" indicator and a
//     "Pin a theme override" button that copies the current *resolved*
//     theme into a draft override the operator can then edit.
//   - When `value` is non-null, per-property color inputs, a font-family
//     text input, and a container-width number input are rendered against
//     the override; a "Reset to tenant defaults" button drops the override.
//   - The component never resolves branding itself — the parent passes the
//     resolved theme in via `resolved`. That keeps the picker decoupled
//     from the TanStack Query lifecycle and easy to test (T110).
//
// Wire shape: `Theme | null` round-trips verbatim to the save endpoint
// (the visual-save body's `theme` field). Null means inherit; an object
// means pinned. See contracts/tenant-api.md.

import { useCallback } from "react"
import type { Theme } from "@/lib/api-types"

type Props = {
  // The row's persisted theme override; null means inherit branding.
  value: Theme | null
  // The resolved theme to display when value is null — the parent computes
  // this via `useEditorTheme(slug, value)`.
  resolved: Theme
  // Fires with the next theme value the row should carry. Null re-enters
  // the inherit-branding state.
  onChange: (next: Theme | null) => void
  // Optional disabled flag — used when the caller has read-only access to
  // the row (forbidden state). Defaults to false.
  disabled?: boolean
}

const CONTAINER_MIN = 320
const CONTAINER_MAX = 800

export function ThemeControls({ value, resolved, onChange, disabled = false }: Props) {
  const inherited = value === null

  const onPin = useCallback(() => {
    if (disabled) return
    // Copy the resolved theme as the operator's starting override — same
    // pattern Phase 6 branding follows when a tenant opens the branding
    // editor.
    onChange({ ...resolved })
  }, [disabled, onChange, resolved])

  const onUnpin = useCallback(() => {
    if (disabled) return
    onChange(null)
  }, [disabled, onChange])

  const patch = useCallback(
    (partial: Partial<Theme>) => {
      if (disabled || value === null) return
      onChange({ ...value, ...partial })
    },
    [disabled, onChange, value],
  )

  return (
    <section
      className="ve-theme-controls"
      data-testid="ve-theme-controls"
      aria-label="Theme controls"
    >
      <header className="ve-theme-controls__header">
        <h3 className="ve-theme-controls__title">Theme</h3>
        {inherited ? (
          <span
            className="ve-theme-controls__inherit-badge"
            data-testid="ve-theme-inherit-badge"
          >
            Using tenant defaults
          </span>
        ) : (
          <span
            className="ve-theme-controls__pinned-badge"
            data-testid="ve-theme-pinned-badge"
          >
            Pinned override
          </span>
        )}
      </header>

      {inherited ? (
        <div className="ve-theme-controls__inherit-body">
          <p className="ve-theme-controls__hint">
            This {/* campaign or template */}row uses the tenant&apos;s
            branding. Pin an override to customize colors, font, and
            container width just for this row.
          </p>
          <button
            type="button"
            className="ve-theme-controls__pin"
            data-testid="ve-theme-pin-override"
            onClick={onPin}
            disabled={disabled}
          >
            Pin a theme override
          </button>
        </div>
      ) : (
        <div className="ve-theme-controls__body" data-testid="ve-theme-pinned-body">
          <ColorField
            label="Text color"
            testId="ve-theme-text-color"
            value={value.textColor}
            onChange={(v) => patch({ textColor: v })}
            disabled={disabled}
          />
          <ColorField
            label="Link color"
            testId="ve-theme-link-color"
            value={value.linkColor}
            onChange={(v) => patch({ linkColor: v })}
            disabled={disabled}
          />
          <ColorField
            label="Button color"
            testId="ve-theme-button-color"
            value={value.buttonColor}
            onChange={(v) => patch({ buttonColor: v })}
            disabled={disabled}
          />
          <ColorField
            label="Button text color"
            testId="ve-theme-button-text-color"
            value={value.buttonTextColor}
            onChange={(v) => patch({ buttonTextColor: v })}
            disabled={disabled}
          />
          <label className="ve-theme-controls__row">
            <span className="ve-theme-controls__label">Font family</span>
            <input
              type="text"
              className="ve-theme-controls__text-input"
              data-testid="ve-theme-font-family"
              value={value.fontFamily}
              onChange={(e) => patch({ fontFamily: e.target.value })}
              disabled={disabled}
              maxLength={256}
            />
          </label>
          <label className="ve-theme-controls__row">
            <span className="ve-theme-controls__label">
              Container width (px)
            </span>
            <input
              type="number"
              className="ve-theme-controls__number-input"
              data-testid="ve-theme-container-width"
              value={value.containerWidth}
              min={CONTAINER_MIN}
              max={CONTAINER_MAX}
              onChange={(e) => {
                const next = Number(e.target.value)
                if (Number.isFinite(next)) {
                  patch({ containerWidth: next })
                }
              }}
              disabled={disabled}
            />
          </label>
          <button
            type="button"
            className="ve-theme-controls__unpin"
            data-testid="ve-theme-reset-defaults"
            onClick={onUnpin}
            disabled={disabled}
          >
            Reset to tenant defaults
          </button>
        </div>
      )}
    </section>
  )
}

type ColorFieldProps = {
  label: string
  value: string
  onChange: (next: string) => void
  testId: string
  disabled?: boolean
}

// ColorField pairs the native `<input type="color">` for visual picking
// with a text input so the operator can type a precise hex / rgb value
// (the native picker only emits `#rrggbb`). Both update the same field.
function ColorField({ label, value, onChange, testId, disabled }: ColorFieldProps) {
  const colorPickerValue = /^#[0-9a-f]{6}$/i.test(value) ? value : "#000000"
  return (
    <label className="ve-theme-controls__row">
      <span className="ve-theme-controls__label">{label}</span>
      <span className="ve-theme-controls__color-pair">
        <input
          type="color"
          className="ve-theme-controls__color-swatch"
          data-testid={`${testId}-swatch`}
          value={colorPickerValue}
          onChange={(e) => onChange(e.target.value)}
          disabled={disabled}
          aria-label={`${label} swatch`}
        />
        <input
          type="text"
          className="ve-theme-controls__color-text"
          data-testid={testId}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          disabled={disabled}
        />
      </span>
    </label>
  )
}

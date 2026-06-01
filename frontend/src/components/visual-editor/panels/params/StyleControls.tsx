// StyleControls — the shared, constrained controls the per-block parameter
// forms compose (feature 017, T024). Every control only permits valid,
// email-safe values (color pickers, bounded numeric steppers, a curated font
// dropdown, an alignment toggle) so the parameters editor can never produce a
// value the validators reject (FR-015). Each field shows an "inherited" state
// when unset and a per-field reset affordance (FR-019).

import { useState } from "react"
import { useTranslation } from "react-i18next"
import type { TFunction } from "i18next"
import type { BlockStyle } from "@/lib/api-types"
import { ALLOWED_FONT_FAMILIES, fontLabel } from "@/server/validate/fonts"

export type StyleField =
  | "backgroundColor"
  | "color"
  | "fontFamily"
  | "fontSize"
  | "fontWeight"
  | "lineHeight"
  | "textAlign"
  | "padding"
  | "borderRadius"
  | "border"

type Props = {
  value: BlockStyle
  fields: ReadonlyArray<StyleField>
  onChange: (patch: Partial<BlockStyle>) => void
  disabled?: boolean
}

// ParamFormProps is the shape every per-block-type parameter form receives from
// BlockParamsPanel: the selected block's type + attrs, its current style, and
// callbacks to patch the style or the type-specific attrs.
export type ParamFormProps = {
  blockType: string
  attrs: Record<string, unknown>
  style: BlockStyle
  onStyleChange: (patch: Partial<BlockStyle>) => void
  onAttrsChange: (patch: Record<string, unknown>) => void
  disabled?: boolean
}

// TextInput is a small labelled text control reused by the type-specific forms
// (button label/href, image alt/href).
export function TextInput({
  label,
  testId,
  value,
  onChange,
  disabled,
}: {
  label: string
  testId: string
  value: string
  onChange: (v: string) => void
  disabled?: boolean
}) {
  return (
    <label className="ve-style-controls__row">
      <span className="ve-style-controls__label">{label}</span>
      <input
        type="text"
        className="ve-style-controls__field"
        data-testid={testId}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        disabled={disabled}
        aria-label={label}
      />
    </label>
  )
}

const FONT_OPTIONS = [...ALLOWED_FONT_FAMILIES]

export function StyleControls({ value, fields, onChange, disabled = false }: Props) {
  const { t } = useTranslation("visualEditor")
  return (
    <div className="ve-style-controls">
      {fields.map((field) => (
        <Field
          key={field}
          field={field}
          value={value}
          onChange={onChange}
          disabled={disabled}
          t={t}
        />
      ))}
    </div>
  )
}

type FieldProps = {
  field: StyleField
  value: BlockStyle
  onChange: (patch: Partial<BlockStyle>) => void
  disabled: boolean
  t: TFunction<"visualEditor">
}

function Field({ field, value, onChange, disabled, t }: FieldProps) {
  switch (field) {
    case "backgroundColor":
    case "color":
    case "border": // border color lives inside the border group below
      if (field === "border") {
        return <BorderGroup value={value} onChange={onChange} disabled={disabled} t={t} />
      }
      return (
        <ColorField
          label={t(`params.fields.${field}`)}
          testId={`ve-param-${field}`}
          value={value[field]}
          onChange={(v) => onChange({ [field]: v })}
          disabled={disabled}
          t={t}
        />
      )
    case "fontFamily":
      return (
        <SelectField
          label={t("params.fields.fontFamily")}
          testId="ve-param-fontFamily"
          value={value.fontFamily}
          options={FONT_OPTIONS.map((f) => ({ value: f, label: fontLabel(f) }))}
          onChange={(v) => onChange({ fontFamily: v })}
          disabled={disabled}
          t={t}
        />
      )
    case "fontWeight":
      return (
        <SelectField
          label={t("params.fields.fontWeight")}
          testId="ve-param-fontWeight"
          value={value.fontWeight == null ? undefined : String(value.fontWeight)}
          options={[
            { value: "400", label: t("params.weight.normal") },
            { value: "700", label: t("params.weight.bold") },
          ]}
          onChange={(v) => onChange({ fontWeight: v ? (Number(v) as 400 | 700) : undefined })}
          disabled={disabled}
          t={t}
        />
      )
    case "fontSize":
      return (
        <NumberField
          label={t("params.fields.fontSize")}
          testId="ve-param-fontSize"
          min={8}
          max={72}
          value={value.fontSize}
          onChange={(v) => onChange({ fontSize: v })}
          disabled={disabled}
          t={t}
        />
      )
    case "lineHeight":
      return (
        <NumberField
          label={t("params.fields.lineHeight")}
          testId="ve-param-lineHeight"
          min={1}
          max={3}
          step={0.1}
          value={value.lineHeight}
          onChange={(v) => onChange({ lineHeight: v })}
          disabled={disabled}
          t={t}
        />
      )
    case "textAlign":
      return <AlignField value={value.textAlign} onChange={(v) => onChange({ textAlign: v })} disabled={disabled} t={t} />
    case "padding":
      return <PaddingGroup value={value} onChange={onChange} disabled={disabled} t={t} />
    case "borderRadius":
      return (
        <NumberField
          label={t("params.fields.borderRadius")}
          testId="ve-param-borderRadius"
          min={0}
          max={48}
          value={value.borderRadius}
          onChange={(v) => onChange({ borderRadius: v })}
          disabled={disabled}
          t={t}
        />
      )
  }
}

function ResetButton({ onReset, disabled, t }: { onReset: () => void; disabled: boolean; t: TFunction<"visualEditor"> }) {
  return (
    <button
      type="button"
      className="ve-style-controls__reset"
      title={t("params.reset")}
      aria-label={t("params.reset")}
      onClick={onReset}
      disabled={disabled}
    >
      ✕
    </button>
  )
}

function ColorField({
  label,
  testId,
  value,
  onChange,
  disabled,
  t,
}: {
  label: string
  testId: string
  value?: string
  onChange: (v: string | undefined) => void
  disabled: boolean
  t: TFunction<"visualEditor">
}) {
  const set = value !== undefined
  return (
    <label className="ve-style-controls__row">
      <span className="ve-style-controls__label">{label}</span>
      <span className="ve-style-controls__field">
        <input
          type="color"
          data-testid={testId}
          value={value ?? "#000000"}
          onChange={(e) => onChange(e.target.value)}
          disabled={disabled}
          aria-label={label}
        />
        {set ? (
          <ResetButton onReset={() => onChange(undefined)} disabled={disabled} t={t} />
        ) : (
          <span className="ve-style-controls__inherited">{t("params.inherited")}</span>
        )}
      </span>
    </label>
  )
}

function NumberField({
  label,
  testId,
  value,
  min,
  max,
  step,
  onChange,
  disabled,
  t,
}: {
  label: string
  testId: string
  value?: number
  min: number
  max: number
  step?: number
  onChange: (v: number | undefined) => void
  disabled: boolean
  t: TFunction<"visualEditor">
}) {
  // While the field is focused we mirror the user's literal keystrokes via a
  // local draft so a multi-digit entry can be typed one digit at a time —
  // clamping the controlled value on every keystroke would corrupt it (e.g.
  // fontSize "18": "1" clamps up to min 8, then "8" → "88" clamps down to max
  // 72). The parent still only ever receives a clamped, in-range value, so the
  // editor can never produce something the validators reject (FR-015). On blur
  // the field snaps back to the committed value.
  const [focused, setFocused] = useState(false)
  const [draft, setDraft] = useState("")
  const display = focused ? draft : value?.toString() ?? ""
  return (
    <label className="ve-style-controls__row">
      <span className="ve-style-controls__label">{label}</span>
      <span className="ve-style-controls__field">
        {/* The reset affordance sits in a fixed-width slot to the LEFT of the
            input and the slot is always reserved, so the input keeps its
            position when the button toggles in/out instead of hopping sideways. */}
        <span className="ve-style-controls__reset-slot">
          {value !== undefined && <ResetButton onReset={() => onChange(undefined)} disabled={disabled} t={t} />}
        </span>
        <input
          type="number"
          data-testid={testId}
          min={min}
          max={max}
          step={step ?? 1}
          value={display}
          placeholder={t("params.inherited")}
          onFocus={() => {
            setDraft(value?.toString() ?? "")
            setFocused(true)
          }}
          onBlur={() => setFocused(false)}
          onChange={(e) => {
            const raw = e.target.value
            setDraft(raw)
            if (raw === "") return onChange(undefined)
            const n = Number(raw)
            if (Number.isNaN(n)) return
            onChange(Math.min(max, Math.max(min, n)))
          }}
          disabled={disabled}
          aria-label={label}
        />
      </span>
    </label>
  )
}

function SelectField({
  label,
  testId,
  value,
  options,
  onChange,
  disabled,
  t,
}: {
  label: string
  testId: string
  value?: string
  options: ReadonlyArray<{ value: string; label: string }>
  onChange: (v: string | undefined) => void
  disabled: boolean
  t: TFunction<"visualEditor">
}) {
  return (
    <label className="ve-style-controls__row">
      <span className="ve-style-controls__label">{label}</span>
      <select
        className="ve-style-controls__field"
        data-testid={testId}
        value={value ?? ""}
        onChange={(e) => onChange(e.target.value === "" ? undefined : e.target.value)}
        disabled={disabled}
        aria-label={label}
      >
        <option value="">{t("params.inherited")}</option>
        {options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </select>
    </label>
  )
}

function AlignField({
  value,
  onChange,
  disabled,
  t,
}: {
  value?: "left" | "center" | "right"
  onChange: (v: "left" | "center" | "right" | undefined) => void
  disabled: boolean
  t: TFunction<"visualEditor">
}) {
  const options: Array<"left" | "center" | "right"> = ["left", "center", "right"]
  return (
    <div className="ve-style-controls__row" role="group" aria-label={t("params.fields.textAlign")}>
      <span className="ve-style-controls__label">{t("params.fields.textAlign")}</span>
      <span className="ve-style-controls__field ve-style-controls__align">
        {options.map((opt) => (
          <button
            key={opt}
            type="button"
            data-testid={`ve-param-align-${opt}`}
            aria-pressed={value === opt}
            className={value === opt ? "is-active" : ""}
            onClick={() => onChange(value === opt ? undefined : opt)}
            disabled={disabled}
          >
            {t(`params.align.${opt}`)}
          </button>
        ))}
      </span>
    </div>
  )
}

function PaddingGroup({
  value,
  onChange,
  disabled,
  t,
}: {
  value: BlockStyle
  onChange: (patch: Partial<BlockStyle>) => void
  disabled: boolean
  t: TFunction<"visualEditor">
}) {
  // `as const` keeps the label entries as literal translation keys so the
  // strongly-typed t() accepts them (a widened `string` would not).
  const sides = [
    ["paddingTop", "params.fields.paddingTop"],
    ["paddingRight", "params.fields.paddingRight"],
    ["paddingBottom", "params.fields.paddingBottom"],
    ["paddingLeft", "params.fields.paddingLeft"],
  ] as const
  return (
    <fieldset className="ve-style-controls__group">
      <legend>{t("params.fields.padding")}</legend>
      {sides.map(([key, label]) => (
        <NumberField
          key={key}
          label={t(label)}
          testId={`ve-param-${key}`}
          min={0}
          max={64}
          value={value[key]}
          onChange={(v) => onChange({ [key]: v })}
          disabled={disabled}
          t={t}
        />
      ))}
    </fieldset>
  )
}

function BorderGroup({
  value,
  onChange,
  disabled,
  t,
}: {
  value: BlockStyle
  onChange: (patch: Partial<BlockStyle>) => void
  disabled: boolean
  t: TFunction<"visualEditor">
}) {
  return (
    <fieldset className="ve-style-controls__group">
      <legend>{t("params.sections.border")}</legend>
      <NumberField
        label={t("params.fields.borderWidth")}
        testId="ve-param-borderWidth"
        min={0}
        max={8}
        value={value.borderWidth}
        onChange={(v) => onChange({ borderWidth: v })}
        disabled={disabled}
        t={t}
      />
      <SelectField
        label={t("params.fields.borderStyle")}
        testId="ve-param-borderStyle"
        value={value.borderStyle}
        options={[
          { value: "solid", label: t("params.borderStyleOption.solid") },
          { value: "dashed", label: t("params.borderStyleOption.dashed") },
          { value: "dotted", label: t("params.borderStyleOption.dotted") },
        ]}
        onChange={(v) => onChange({ borderStyle: v as BlockStyle["borderStyle"] })}
        disabled={disabled}
        t={t}
      />
      <ColorField
        label={t("params.fields.borderColor")}
        testId="ve-param-borderColor"
        value={value.borderColor}
        onChange={(v) => onChange({ borderColor: v })}
        disabled={disabled}
        t={t}
      />
    </fieldset>
  )
}

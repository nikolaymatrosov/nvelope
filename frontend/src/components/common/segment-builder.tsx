// Recursive segment / query builder (FR-015, research.md Decision 9). Emits the
// backend `SegmentNode` tree with lowerCamelCase keys, generated from
// api/openapi/tenant.yaml and re-exported by src/lib/api-types.ts.

import { PlusIcon, Trash2Icon } from "lucide-react"
import { useTranslation } from "react-i18next"
import type {
  Conjunction,
  FieldName,
  SegmentNode,
  SegmentOp,
} from "@/lib/api-types"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

const OPS: Array<SegmentOp> = [
  "eq",
  "neq",
  "exists",
  "contains",
  "gt",
  "lt",
  "gte",
  "lte",
]
const FIELDS: Array<FieldName> = ["email", "name", "state"]

type LeafKind = "field" | "attr" | "member"

export function emptyGroup(): SegmentNode {
  return { conj: "and", children: [] }
}

function emptyField(): SegmentNode {
  return { field: { field: "email", op: "eq", value: "" } }
}

function isGroup(node: SegmentNode): boolean {
  return node.children !== undefined || node.conj !== undefined
}

function leafKind(node: SegmentNode): LeafKind {
  if (node.attr) return "attr"
  if (node.member) return "member"
  return "field"
}

function emptyLeaf(kind: LeafKind): SegmentNode {
  if (kind === "attr") return { attr: { key: "", op: "eq", value: "" } }
  if (kind === "member") return { member: { listId: "" } }
  return emptyField()
}

function LeafEditor({
  node,
  onChange,
}: {
  node: SegmentNode
  onChange: (next: SegmentNode) => void
}) {
  const { t } = useTranslation()
  const kind = leafKind(node)
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Select
        value={kind}
        onValueChange={(v) => onChange(emptyLeaf(v as LeafKind))}
      >
        <SelectTrigger className="w-36">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            <SelectItem value="field">
              {t("segmentBuilder.leafKind.field")}
            </SelectItem>
            <SelectItem value="attr">
              {t("segmentBuilder.leafKind.attr")}
            </SelectItem>
            <SelectItem value="member">
              {t("segmentBuilder.leafKind.member")}
            </SelectItem>
          </SelectGroup>
        </SelectContent>
      </Select>

      {kind === "field" && node.field && (
        <>
          <Select
            value={node.field.field}
            onValueChange={(v) =>
              onChange({ field: { ...node.field!, field: v } })
            }
          >
            <SelectTrigger className="w-32">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                {FIELDS.map((f) => (
                  <SelectItem key={f} value={f}>
                    {f}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
          <OpSelect
            op={node.field.op}
            onChange={(op) => onChange({ field: { ...node.field!, op } })}
          />
          {node.field.op !== "exists" && (
            <Input
              className="w-40"
              placeholder={t("segmentBuilder.valuePlaceholder")}
              value={node.field.value ?? ""}
              onChange={(e) =>
                onChange({ field: { ...node.field!, value: e.target.value } })
              }
            />
          )}
        </>
      )}

      {kind === "attr" && node.attr && (
        <>
          <Input
            className="w-36"
            placeholder={t("segmentBuilder.attrKeyPlaceholder")}
            value={node.attr.key}
            onChange={(e) =>
              onChange({ attr: { ...node.attr!, key: e.target.value } })
            }
          />
          <OpSelect
            op={node.attr.op}
            onChange={(op) => onChange({ attr: { ...node.attr!, op } })}
          />
          {node.attr.op !== "exists" && (
            <Input
              className="w-40"
              placeholder={t("segmentBuilder.valuePlaceholder")}
              value={String(node.attr.value ?? "")}
              onChange={(e) =>
                onChange({ attr: { ...node.attr!, value: e.target.value } })
              }
            />
          )}
        </>
      )}

      {kind === "member" && node.member && (
        <>
          <Input
            className="w-44"
            placeholder={t("segmentBuilder.listIdPlaceholder")}
            value={node.member.listId}
            onChange={(e) =>
              onChange({ member: { ...node.member!, listId: e.target.value } })
            }
          />
          <Input
            className="w-36"
            placeholder={t("segmentBuilder.statusPlaceholder")}
            value={node.member.status ?? ""}
            onChange={(e) =>
              onChange({
                member: {
                  ...node.member!,
                  status: e.target.value as NonNullable<
                    SegmentNode["member"]
                  >["status"],
                },
              })
            }
          />
        </>
      )}
    </div>
  )
}

function OpSelect({
  op,
  onChange,
}: {
  op: SegmentOp
  onChange: (op: SegmentOp) => void
}) {
  const { t } = useTranslation()
  return (
    <Select value={op} onValueChange={(v) => onChange(v as SegmentOp)}>
      <SelectTrigger className="w-44">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectGroup>
          {OPS.map((o) => (
            <SelectItem key={o} value={o}>
              {t(`segmentBuilder.op.${o}`)}
            </SelectItem>
          ))}
        </SelectGroup>
      </SelectContent>
    </Select>
  )
}

function GroupEditor({
  node,
  onChange,
  depth,
}: {
  node: SegmentNode
  onChange: (next: SegmentNode) => void
  depth: number
}) {
  const { t } = useTranslation()
  const children = node.children ?? []

  function setChild(index: number, next: SegmentNode) {
    const copy = children.slice()
    copy[index] = next
    onChange({ ...node, children: copy })
  }
  function removeChild(index: number) {
    onChange({ ...node, children: children.filter((_, i) => i !== index) })
  }
  function addChild(next: SegmentNode) {
    onChange({ ...node, children: [...children, next] })
  }

  return (
    <div
      className={cn(
        "flex flex-col gap-3 rounded-lg border p-3",
        depth > 0 && "bg-muted/40",
      )}
    >
      <div className="flex items-center gap-2">
        <span className="text-sm text-muted-foreground">
          {t("segmentBuilder.matchPrefix")}
        </span>
        <Select
          value={node.conj ?? "and"}
          onValueChange={(v) => onChange({ ...node, conj: v as Conjunction })}
        >
          <SelectTrigger className="w-24">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem value="and">
                {t("segmentBuilder.matchAll")}
              </SelectItem>
              <SelectItem value="or">
                {t("segmentBuilder.matchAny")}
              </SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>
        <span className="text-sm text-muted-foreground">
          {t("segmentBuilder.matchSuffix")}
        </span>
      </div>

      {children.map((child, index) => (
        <div key={index} className="flex items-start gap-2">
          <div className="flex-1">
            {isGroup(child) ? (
              <GroupEditor
                node={child}
                onChange={(next) => setChild(index, next)}
                depth={depth + 1}
              />
            ) : (
              <LeafEditor
                node={child}
                onChange={(next) => setChild(index, next)}
              />
            )}
          </div>
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            aria-label={t("segmentBuilder.removeCondition")}
            onClick={() => removeChild(index)}
          >
            <Trash2Icon />
          </Button>
        </div>
      ))}

      <div className="flex gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => addChild(emptyField())}
        >
          <PlusIcon /> {t("segmentBuilder.addCondition")}
        </Button>
        {depth < 2 && (
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => addChild(emptyGroup())}
          >
            <PlusIcon /> {t("segmentBuilder.addGroup")}
          </Button>
        )}
      </div>
    </div>
  )
}

export function SegmentBuilder({
  value,
  onChange,
}: {
  value: SegmentNode
  onChange: (next: SegmentNode) => void
}) {
  return <GroupEditor node={value} onChange={onChange} depth={0} />
}

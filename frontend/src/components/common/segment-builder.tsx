// Recursive segment / query builder (FR-015, research.md Decision 9). Emits the
// backend `Node` tree with PascalCase keys — the audience domain struct has no
// json tags, so the API decodes `Conj`, `Children`, `Field`, `Attr`, `Member`.

import { PlusIcon, Trash2Icon } from "lucide-react"
import { useTranslation } from "react-i18next"
import type {
  Conjunction,
  FieldName,
  Node,
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

export function emptyGroup(): Node {
  return { Conj: "and", Children: [] }
}

function emptyField(): Node {
  return { Field: { Field: "email", Op: "eq", Value: "" } }
}

function isGroup(node: Node): boolean {
  return node.Children !== undefined || node.Conj !== undefined
}

function leafKind(node: Node): LeafKind {
  if (node.Attr) return "attr"
  if (node.Member) return "member"
  return "field"
}

function emptyLeaf(kind: LeafKind): Node {
  if (kind === "attr") return { Attr: { Key: "", Op: "eq", Value: "" } }
  if (kind === "member") return { Member: { ListID: "", Status: "subscribed" } }
  return emptyField()
}

function LeafEditor({
  node,
  onChange,
}: {
  node: Node
  onChange: (next: Node) => void
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

      {kind === "field" && node.Field && (
        <>
          <Select
            value={node.Field.Field}
            onValueChange={(v) =>
              onChange({ Field: { ...node.Field!, Field: v as FieldName } })
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
            op={node.Field.Op}
            onChange={(op) => onChange({ Field: { ...node.Field!, Op: op } })}
          />
          {node.Field.Op !== "exists" && (
            <Input
              className="w-40"
              placeholder={t("segmentBuilder.valuePlaceholder")}
              value={node.Field.Value}
              onChange={(e) =>
                onChange({ Field: { ...node.Field!, Value: e.target.value } })
              }
            />
          )}
        </>
      )}

      {kind === "attr" && node.Attr && (
        <>
          <Input
            className="w-36"
            placeholder={t("segmentBuilder.attrKeyPlaceholder")}
            value={node.Attr.Key}
            onChange={(e) =>
              onChange({ Attr: { ...node.Attr!, Key: e.target.value } })
            }
          />
          <OpSelect
            op={node.Attr.Op}
            onChange={(op) => onChange({ Attr: { ...node.Attr!, Op: op } })}
          />
          {node.Attr.Op !== "exists" && (
            <Input
              className="w-40"
              placeholder={t("segmentBuilder.valuePlaceholder")}
              value={String(node.Attr.Value ?? "")}
              onChange={(e) =>
                onChange({ Attr: { ...node.Attr!, Value: e.target.value } })
              }
            />
          )}
        </>
      )}

      {kind === "member" && node.Member && (
        <>
          <Input
            className="w-44"
            placeholder={t("segmentBuilder.listIdPlaceholder")}
            value={node.Member.ListID}
            onChange={(e) =>
              onChange({ Member: { ...node.Member!, ListID: e.target.value } })
            }
          />
          <Input
            className="w-36"
            placeholder={t("segmentBuilder.statusPlaceholder")}
            value={node.Member.Status}
            onChange={(e) =>
              onChange({ Member: { ...node.Member!, Status: e.target.value } })
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
  node: Node
  onChange: (next: Node) => void
  depth: number
}) {
  const { t } = useTranslation()
  const children = node.Children ?? []

  function setChild(index: number, next: Node) {
    const copy = children.slice()
    copy[index] = next
    onChange({ ...node, Children: copy })
  }
  function removeChild(index: number) {
    onChange({ ...node, Children: children.filter((_, i) => i !== index) })
  }
  function addChild(next: Node) {
    onChange({ ...node, Children: [...children, next] })
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
          value={node.Conj ?? "and"}
          onValueChange={(v) => onChange({ ...node, Conj: v as Conjunction })}
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
  value: Node
  onChange: (next: Node) => void
}) {
  return <GroupEditor node={value} onChange={onChange} depth={0} />
}

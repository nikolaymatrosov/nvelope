# Contract: Three-Pane Editor UI Behavior

**Feature**: 017-three-pane-visual-editor | **Date**: 2026-05-31

The user-facing contract for the three-pane editor. These are testable
behavioral guarantees (component/Storybook tests) — not visual styling specs.

## Layout shell (`ThreePaneEditor`)

| ID | Guarantee | FR |
|----|-----------|-----|
| L1 | Renders three regions in order: left structure panel, center canvas (`VisualEmailEditor`), right parameters panel. | FR-001 |
| L2 | Shown only on the visual surface; code-only / legacy rows render without the side panels. | FR-003 |
| L3 | Left and right panels each have an always-available collapse control and, when collapsed, an always-available re-expand affordance. | FR-020 |
| L4 | Collapsing a panel reflows the canvas to occupy the freed width; re-expanding restores the panel's prior width with content intact. | FR-020 |
| L5 | Collapsed/expanded state and panel widths persist across editor reopen (same browser). | FR-021 |
| L6 | At viewport ≤ ~1024 px, side panels default collapsed and open as overlays; the canvas never drops below its minimum usable width (no horizontal overflow). | FR-022 / SC-006 |
| L7 | All panel chrome strings come from the `visualEditor` i18n namespace (en + ru parity). | FR-004 |

## Selection model (`useBlockSelection`)

| ID | Guarantee | FR |
|----|-----------|-----|
| S1 | At most one block is selected at any time. | FR-002 |
| S2 | Selecting a block on the canvas highlights its outline entry and loads its params. | FR-002 |
| S3 | Selecting an outline entry selects the block, scrolls it into view on the canvas, and loads its params. | FR-007 |
| S4 | After the selected block is deleted, selection clears and the params panel shows its empty state. | edge case |
| S5 | Selecting a block nested in a column selects exactly that block (not its container) in all three panes. | edge case |

## Left panel — structure outline (`StructureOutline`)

| ID | Guarantee | FR |
|----|-----------|-----|
| O1 | Renders one indented entry per block, mirroring document nesting including per-column children. | FR-005 |
| O2 | Each entry shows its block type and a content-derived label. | FR-006 |
| O3 | The selected block's entry is visually highlighted regardless of how it was selected. | FR-008 |
| O4 | Dragging an entry to a valid position reorders the block (with nested children for containers), matching the canvas drag-handle result. | FR-009 |
| O5 | A reorder to an invalid target (e.g. a columns container into its own column) is rejected; the document is unchanged. | edge case |
| O6 | Per-entry delete and duplicate actions update the canvas, outline, and produced output. | FR-010 |
| O7 | Container entries collapse/expand to hide/show their children. | FR-011 |

## Right panel — block parameters (`BlockParamsPanel`)

| ID | Guarantee | FR |
|----|-----------|-----|
| P1 | With a block selected, shows exactly that block type's applicable parameter set, populated with current values. | FR-012 |
| P2 | With nothing selected, shows a neutral empty/prompt state (no stale controls). | FR-012 |
| P3 | Changing a parameter applies to the selected block only, updates the canvas live (< 1 s), and does not change the document Theme or other blocks. | FR-014 / SC-001 |
| P4 | Every control constrains input to valid, email-safe values (color picker, bounded numeric stepper/slider, font dropdown, alignment toggle) — no free-form entry that could fail validation. | FR-015 |
| P5 | Offers per-field and whole-block "reset to default," restoring inheritance from the Theme; the panel distinguishes explicitly-set from inherited. | FR-019 |
| P6 | Block types with no/few style params (divider, rawHtml, codeBlock) show only their applicable controls; rawHtml directs to "edit as HTML." | edge case |
| P7 | Parameter changes persist via the existing save flow and round-trip losslessly on reopen. | FR-017 / SC-002 |

## Failure surfacing

| ID | Guarantee | FR |
|----|-----------|-----|
| F1 | Any failed operation through the panels (apply parameter, reorder, delete, save) surfaces a clear named reason, never a silent failure or generic error. | FR-024 |
| F2 | A save rejected for an invalid style (`invalid_style`) names the offending block/field. | FR-024 |
| F3 | Concurrent-edit conflicts surface the existing reload / force-overwrite affordance (`409 stale_row`). | edge case |

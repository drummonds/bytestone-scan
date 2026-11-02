# 104 — gobank-workflow 0.17.1: step modes, two-way diagrams (#136)

**Status:** in progress

## Why

The scanner is four gobank-workflow releases behind. 0.15.0 broke two
things it uses: `Step.Many` became `Step.Mode`, which distinguishes a
one-at-a-time loop from an all-at-once fan-out, and `diagram.Definition`
takes a direction so that a definition is drawn across for a wide screen
and down for a phone. The engine's view now looks for `<type>.down.svg`
beside each `<type>.svg`. Nothing else outdated is a direct dependency.

## What changes

- `go.mod`: gobank-workflow 0.13.0 → 0.17.1 (Go stays at 1.27.3).
- `internal/batch/definition.go`: the batch's *scans* step is
  `OneAtATime` (the operator starts each scan; the step waits for each),
  the scan's *photos* step is `AllAtOnce`; each says in `Given` what its
  children are handed.
- `internal/web/diagrams/gen`: draws each definition twice, `<type>.svg`
  across and `<type>.down.svg` down; the embed takes both.
- The definitions test asks for the down picture as well as the across
  one.

## Acceptance

| Definition page | Picture | Outcome |
|---|---|---|
| any scanner workflow | `diagram.svg` | 200, an SVG naming every step |
| any scanner workflow | `diagram.down.svg` | 200, an SVG naming every step |
| scan_batch | *scans* step | drawn with a loop arrow, "one at a time" |
| scan | *photos* step | drawn as stacked boxes, all at once |

## Out of scope

- Progress dots on the batch page (`view.Views.Progress`, 0.15.0): a
  feature of its own.
- A `Joins` on the scan's *photos* step: no scan step waits for the
  photos; the batch's *scans* step does, through Standing. The view's
  "nothing waits for the children" tag on that step is true at the scan's
  level.

## Decisions

none

## Done when

- [x] tests express the acceptance table and pass
- [x] changelog fragment
- [ ] issue closed with a link to the release

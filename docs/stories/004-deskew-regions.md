# 004 — Deskew regions at the preview (#4)

**Status:** done

## Why

A region was a rectangle square to the bed. On a print lying at an angle
its edges run mostly over the bed, so moving them at confirm, or from the
quality page, says little about the print. A region square to the print
makes its edges the print's own, and gives story 003's rendition the
print's angle and extent to start from.

## What changes

- A region is square to its print: a centre on the bed, a size along the
  print's own axes, and a signed angle, clockwise as the preview is seen.
- The close-up at confirm is drawn turned so the print sits square; the
  arrows move the print's own edges, in and out along the print's axes.
- The annotated preview outlines each region at its angle.
- The detail pass scans each region's bounds: the smallest rectangle
  square to the bed that covers it, within the bed. The master covers the
  bounds and is not rotated (format statement: masters are never altered).
- The Side records the region and the bounds the master covers.
- The preview is 150 ppi (story 002's fix): the 4990 scans whole 8 px
  blocks across, which lost 2 mm of the bed's right edge at 75 ppi.

## Acceptance

### Skew

| Skew found | Close-up | Arrows move | Detail pass scans | Recorded on the Side |
|---|---|---|---|---|
| under 0.1° | as scanned | the bed's edges | the region | angle 0 |
| 0.1° to 10° | turned square | the print's own edges | the region's bounds | the signed angle, the region in the print's frame |
| over 10° | as scanned, flagged | none: straighten the print and rescan the preview | the region | angle 0 |
| not rectangular | turned by its smallest enclosing rectangle, flagged | the print's own edges | the region's bounds | as for its skew |

A print is flagged for review when it is over 10° from square, not
rectangular, or more than one print. Skew between 0.1° and 10° is no
longer flagged: the region takes it up.

### Moving an edge

| | |
|---|---|
| outwards | the edge moves along the print's axis, the opposite edge stays |
| outwards at the bed | stops when the edge's midpoint reaches the bed's edge; no arrow once it is there |
| inwards | refused below 20 mm a side |

## Out of scope

Rotating anything: the master is never rotated, and the rendition is story
003. Measuring the angle on the master at full resolution is story 003's;
this story's angle is the preview's.

## Decisions

- Angle sign: positive turns the print's top edge clockwise as the preview
  is seen (y down), the convention the finder's tests draw prints with.
- A print turned more than 10° is not deskewed: its bounds would scan
  much bed, and the operator straightening it costs one preview.
- "Not rectangular" turns by the smallest enclosing rectangle, which is
  what the finder already measures; on a print torn at a corner it lies
  along the longest straight edge.

# 114 — Set the turn at confirm so the picture is the right way up before the fronts are scanned (#154)

**Status:** built 2026-11-05

## Why

Story 112 lets the operator turn a rendition by quarter turns, but only
at the quality check, once the fronts and backs are scanned. At confirm
the operator is already looking at every print's close-up, and a print
laid on its side or upside down is plain there; the quality check is a
second pass over every print just to say what was seen at the first,
and the first renditions of the scan are made the wrong way up. The
quality gate is too late: the turn should be said where the print is
first seen, and the quality check left to check.

## What changes

- A found print records a turn: the quarter turns clockwise its
  rendition is to be given so the picture is the right way up, `turn` in
  prints.yaml, 0 left out. Moving an edge keeps it; splitting a region
  gives every part the whole's turn; rescanning the preview loses it with
  the regions.
- `batch.Scan.TurnPrint(n, by)`: adds a quarter turn to print n,
  clockwise or anticlockwise, while the prints are editable, as the
  arrows are; refused once confirmed.
- The fronts pass copies the found print's turn onto the front Side at
  capture, so photo.yaml, the bag and the first renditions carry it. A
  quality-check turn of the front (story 112) is written back to the found
  print too, so a reshape, which captures afresh from prints.yaml, keeps
  whichever turn was last said.
- The confirm page: *Turn ↺* and *Turn ↻* on each print's close-up,
  posting to `batches/{id}/scans/{n}/turn` with the print and the
  direction, beside Remove and Split. The print's heading and the prints
  table say the turn.
- The close-up is shown turned, as the rendition will be, and its rulers
  and arrows are laid round it as shown: the arrows at the top move the
  edge seen at the top (the print's left edge at a quarter turn
  clockwise) and point up and down; the top ruler runs along whichever of
  the region's axes lies that way, backwards where the turn reverses it,
  and sets the edges seen at its ends. The move posted still names the
  print's own edge. The back's close-up, in the same view, turns with it;
  so do the quality page's margin arrows, beside the turned rendition.
- The back's turn is unchanged: set at the quality check, where the back
  is first seen.
- The glossary's Turn says it is said at confirm or the quality check.

## Acceptance

| Print's turn at confirm | Then | Front Side | Renditions |
|---|---|---|---|
| none | fronts scanned | turn 0 | as story 003 |
| once clockwise | fronts scanned | turn 90 | portrait display from a landscape print |
| clockwise then anticlockwise | fronts scanned | turn 0 | as none |
| once clockwise, then the right edge moved 3 mm | fronts scanned | turn 90 | turned |
| once clockwise, region split by hand into 2 | fronts scanned | both turn 90 | both turned |
| once clockwise, preview rescanned | — | the found prints have no turn | — |
| once clockwise, prints confirmed, turned again | — | refused: the prints are confirmed | — |
| none; turned 90° at the quality check; a margin arrow reshapes | recaptured | turn 90 kept | turned |
| by 45° | — | refused | — |

| Scan | Turn buttons on the confirm page |
|---|---|
| at confirm, print deskewable | Turn ↺ and Turn ↻ on each print's close-up; the heading says the turn once there is one |
| at confirm, print over 10° | still offered: the turn is the rendition's, not the deskew's |

| Print's turn | Close-up of a landscape print | Arrows at the top | Top ruler sets |
|---|---|---|---|
| none | landscape, as scanned | the top edge's, ↑ out | left and right edges |
| 90° clockwise | portrait, the print's left edge at the top | the left edge's, ↑ out | top and bottom edges, backwards |
| 180° | landscape, upside down | the bottom edge's | left and right edges, backwards |
| 90° anticlockwise | portrait, the right edge at the top | the right edge's | top and bottom edges |
| confirmed | none, as the arrows |

## Out of scope

Turning the whole-bed preview: it shows the bed as it lies. Telling
which way up a picture is from its content. The back's turn at the backs
preview.

## Decisions

- Reverses story 112's "set at the quality check, not at confirm": the
  turn is now a fact of the found print as much as its region is, and is
  carried into photo.yaml at capture as the region is. Two records of one
  fact, prints.yaml's and photo.yaml's, with the front's turn kept the same
  in both by the quality-check Turn writing back.

## Done when

- [x] tests express the acceptance table and pass
- [x] glossary updated; changelog fragment
- [ ] issue closed with a link to the release

# 112 — Turn a rendition by quarter turns so the picture is the right way up (#150)

**Status:** built 2026-11-04

## Why

Prints are laid on the bed to fit it, not the right way up: a landscape
print goes sideways beside a portrait one, and a print with writing on
its back is turned over about whichever edge the operator's hand took.
The rendition deskews the print to the nearest right angle, so a print
scanned on its side is shown on its side, and one scanned upside down
stays so. Nothing says which way up the picture is. The operator can see
it on the quality page; the tool should let them say so, and record it
where the renditions are made from, so every later recipe shows the
picture the right way up.

## What changes

- A Side records a turn: the quarter turns clockwise its rendition is
  given so the picture is the right way up, in degrees (0, 90, 180, 270),
  `turn` in photo.yaml, 0 left out. The front and the back each have
  their own: a back is written on any way round.
- `rendition.Develop` turns the deskewed crop by the side's turn, then
  fits it to the display size: a landscape print turned 90° is 1080 tall.
  The recipe records the turn beside the skew.
- `batch.Scan.Turn(id, side, by)`: adds a quarter turn to that side,
  clockwise or anticlockwise, and makes its renditions again; refused once
  the photo is accepted, since photo.yaml is packaged from then.
- The quality page: two buttons under the front's rendition and the
  back's, *Turn ↺* and *Turn ↻*, posting to `photos/{id}/turn` with the
  side and the direction, while the scan waits at the quality check, as
  the margins' arrows are. The rendition's caption says the turn.
- The glossary gains Turn.

## Acceptance

| Side's turn | Print on the bed | Rendition | Recipe |
|---|---|---|---|
| none | 120 × 90 mm, square | as story 003: deskewed, 1440 × 1080 | turn 0 |
| 90° clockwise | 120 × 90 mm, square | the print's left edge at the top, 810 × 1080 | turn 90 |
| twice clockwise | 120 × 90 mm, square | upside down, 1440 × 1080 | turn 180 |
| once anticlockwise | any | as three times clockwise | turn 270 |
| clockwise then anticlockwise | any | as none | turn 0 |
| 90° clockwise | turned 2.3° | deskewed by 2.3° and then turned 90°: no lid in the corners | skew 2.3, turn 90 |
| 90° clockwise | turned 12° | cropped square to the bed, as story 003, then turned 90°; the over-10° flag stays | skew 0, turn 90 |

| Photo | Turn buttons | Turn posted |
|---|---|---|
| at the quality check, back scanned | under the front's rendition and the back's | the side turns, its renditions made again, the other side untouched |
| at the quality check, back blank | under the front's only | as above |
| accepted | none | refused |
| moved by a margin arrow afterwards | — | a reshape is a new capture: photo.yaml is written afresh and the turn is said again |

## Out of scope

Rotation finer than a quarter turn by hand (story 003's 0.1° steps),
with crop and level nudges: still a story of its own. Telling which way
up a picture is from its content. The master's XMP: it holds what is
fixed at capture, and the turn is said after. Setting the turn at
confirm, from the preview.

## Decisions

- The turn is a fact of the photo, kept on the Side in photo.yaml and so
  in the bag, not in the recipe alone: renditions are thrown away and made
  again, and the picture's way up must survive that. The recipe records
  the turn it applied, as it records the skew.
- Degrees clockwise, as the angle convention is (story 004), so the
  rendition's whole rotation reads as skew plus turn.
- Set at the quality check, where the rendition is seen, not at confirm:
  one place, and the rendition is what is being judged. A reshape loses
  it, which is rare and cheap to say again; carrying it through prints.yaml
  would make two records of one fact.

## Done when

- [x] tests express the acceptance table and pass
- [x] glossary updated; changelog fragment
- [ ] issue closed with a link to the release

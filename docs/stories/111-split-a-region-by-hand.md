# 111 — Split a region by hand (#147)

**Status:** built 2026-11-03

## Why

The finder gives one region to prints that touch, and offers to split it
only when it can tell the prints apart itself. A contact print, a strip
of frames or two prints butted together are one region with nothing to
split: the operator's only way to two photos is to remove the region
and rescan, or to narrow it to one print and lose the other. The
operator can see where the cut goes; the tool should let them make it
and then move the edges as they do now.

## What changes

- `find.Print.Divide(across, down)`: the region cut into across × down
  equal parts along its own axes, numbered in rows. Each part's region is
  its cell; its print keeps the original print's edge on the outside and
  sits the margin inside the cell on a cut edge, as a moved edge does.
  Parts keep the skew, are not flagged for review, and are refused when
  a cell would be under the smallest print, 20 mm.
- `batch.Scan.SplitInto(n, across, down)`: replaces print n's region
  with the parts, until the prints are confirmed, as the other edits.
- The region editor at confirm: on every region a form, *Split into
  [2] across and [1] down*, beside the finder's own Split when it has
  one; it posts to `split` with `across` and `down`, which the finder's
  button leaves out.

## Acceptance

| Region | Across × down | Outcome |
|---|---|---|
| 120 × 60 mm, print 1 mm inside | 2 × 1 | two 60 × 60 regions side by side; part 1's print from the original's left edge to 0.5 mm short of the cut, part 2's from 0.5 mm past the cut to the original's right edge |
| any | 1 × 1, or 0 | refused: nothing to split |
| 120 mm across | 7 × 1 | refused: a part would be under 20 mm |
| turned 3° | 2 × 2 | four parts at the same angle, numbered in rows |
| confirmed | any | refused, as every edit is |
| turned over 10° | – | no form: straighten on the bed, as the arrows say |

## Out of scope

Finding the frames of a contact print from the preview. Unequal cuts:
the arrows move the edges afterwards. Splitting after confirm, from
the quality page.

## Decisions

none

## Done when

- [x] tests express the acceptance table and pass
- [x] changelog entry
- [ ] issue closed with a link to the release

# 003 — Display images: crop, deskew, levels (#3)

**Status:** proposed (revised 2026-10-23: sizes, where renditions are
made, what is split out)

## Why

The masters are archival: the print plus a margin, never rotated, never
corrected. What i.bytestone.uk shows is a rendition made from a master,
and it is also the best way to judge whether the process is good, since it
is what people will see. A rendition is never a master: it can be thrown
away and made again whenever the processing improves.

## What changes

- A **develop** step in the batch workflow, after the detail pass and
  before the quality check: each master gets its renditions, so the
  operator judges the picture people will see beside the master.
- A **recipe version**, one constant in the code. Renditions are kept by
  `<photo-id>/<recipe version>/`; a new version makes new renditions and
  leaves the old on record. Re-developing photos already archived is a
  story of its own (see Out of scope).
- **Crop** to the print: no margin, no lid.
- **Deskew**: the print's edges found on the master at full resolution,
  starting from the angle the region recorded, and the image rotated so
  they are square.
- **Levels**: black and white points from the print's histogram, ignoring
  the clipped lid; one curve for all three channels.
- Renditions are sRGB JPEGs, stored under the staging root beside the
  archive, never in the bag.
- The quality page shows the display rendition beside the master.

## Acceptance

### Develop

| Master | Renditions |
|---|---|
| each master of the batch | `display.jpg`, `thumb.jpg`, `tiny.jpg`, `recipe.yaml` under `renditions/<photo-id>/<recipe version>/` |
| already developed at this version | left as they are |
| developed at an older version | that version's renditions kept; this version's made beside them |

| Step | Kind | On redo |
|---|---|---|
| develop | run, after detail pass | the detail pass redone clears the renditions of this version: the master changed |

### Renditions

| File | Size | JPEG |
|---|---|---|
| `display.jpg` | fits within 1920 × 1080 (a portrait print is 1080 tall), never scaled up | quality 90, sRGB |
| `thumb.jpg` | 200 px on the long edge | quality 85 |
| `tiny.jpg` | 64 px on the long edge | quality 80 |
| `recipe.yaml` | recipe version; the measured skew; the crop in master pixels; the black and white points; the master's sha256 | — |

### Deskew

| Print on the master | Rendition |
|---|---|
| square to the bed | not rotated |
| skewed 0.1° to 10° | rotated square, measured to 0.1° |
| skewed over 10° | not rotated; flagged for the operator (find already asks for it to be straightened) |
| not rectangular (torn, trimmed) | rotated by its longest straight edge; flagged |

| Measured against the region's angle | |
|---|---|
| within 0.5° | the master's measurement is used |
| further | the master's measurement is used and the difference flagged |

### Crop

| | |
|---|---|
| margin and lid | none left after rotation: the largest rectangle square to the print within its edges |
| a print against the bed's edge | cropped at the bed's edge |

### Levels

| | |
|---|---|
| black point | 0.1% of the print's samples darker |
| white point | 0.1% lighter, the lid excluded |
| sepia and colour | the balance kept: one curve for all three channels |
| already full range | unchanged |

### Quality page

| Shows | |
|---|---|
| the rendition | `display.jpg`, beside the master, with its size and the recipe version |
| flags | skew over 10°, not rectangular, measured angle far from the region's |

## Out of scope

- **Operator adjustments** (rotation in 0.1° steps, crop nudges,
  brightness, contrast, recorded with the rendition): a story of its own
  once the automatic rendition has been judged on real prints (#7's
  recording first).
- **Re-developing archived photos** at a new recipe version: a develop
  workflow per photo, keyed `<photo-id>@<recipe version>`. Nothing reads
  renditions yet, so it waits for publishing (roadmap 9).
- Dust removal, colour from an ICC profile (roadmap 11), backs,
  publishing.

## Decisions

- Rotation and levels never touch the master (format statement:
  Principles). Deskew needs the master's full resolution, so it measures
  the edges there rather than trusting the preview's angle, which is good
  to about 0.4 mm.
- Renditions are made in the batch, not in a workflow of their own, so
  the quality check judges what people will see. The per-photo workflow
  is for later recipes.
- Until there is an ICC profile the master's colour is taken as sRGB;
  renditions are remade once there is one.
- Renditions live beside the archive, not in the bag: a bag is never
  altered, and a rendition is made again whenever the recipe improves.

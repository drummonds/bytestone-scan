# 003 — Display images: crop, deskew, levels (#3)

**Status:** proposed

## Why

The masters are archival: the print plus a margin, never rotated, never
corrected. What i.bytestone.uk shows is a rendition made from a master,
and it is also the best way to judge whether the process is good, since it
is what people will see. A rendition is never a master: it can be thrown
away and made again whenever the processing improves.

## What changes

- A **develop** workflow per photo, keyed `<photo-id>@<recipe version>`:
  a new recipe version makes new renditions and leaves the old on record.
- **Crop** to the print: no margin, no lid.
- **Deskew**: the print's edges found on the master at full resolution,
  the image rotated so they are square.
- **Levels**: black and white points from the print's histogram, ignoring
  the clipped lid.
- **Adjustments** by the operator, recorded with the rendition, never on
  the master: rotation in 0.1° steps, crop nudges, brightness and
  contrast.
- Renditions are sRGB JPEGs: a display image of 2048 px on its long edge
  and a 400 px thumbnail, stored beside the bag, not in it.
- The quality page shows the rendition beside the master.

## Acceptance

### Deskew

| Print on the master | Rendition |
|---|---|
| square to the bed | not rotated |
| skewed 0.1° to 10° | rotated square, measured to 0.1° |
| skewed over 10° | not rotated; flagged for the operator |
| not rectangular (torn, trimmed) | rotated by its longest straight edge; flagged |

### Crop

| | |
|---|---|
| margin and lid | none left after rotation |
| a print against the bed's edge | cropped at the bed's edge |

### Levels

| | |
|---|---|
| black point | 0.1% of the print's samples darker |
| white point | 0.1% lighter, the lid excluded |
| sepia and colour | the balance kept: one curve for all three channels |

### Adjustments

| Adjustment | Step | Recorded as |
|---|---|---|
| rotation | ±0.1°, ±1° | degrees added to the measured skew |
| crop | ±0.5 mm per edge | mm per edge |
| brightness, contrast | ±5% | the curve's parameters |

### Renditions

| File | Size | Where |
|---|---|---|
| `display.jpg` | 2048 px long edge, quality 90, sRGB | `renditions/<photo-id>/<recipe>/` |
| `thumb.jpg` | 400 px long edge | likewise |
| `recipe.yaml` | the measured skew, crop, levels and adjustments | likewise |

## Out of scope

Dust removal, colour from an ICC profile (story: colour profile), backs,
publishing.

## Decisions

- Rotation and levels never touch the master (format statement:
  Principles). Deskew needs the master's full resolution, so it measures
  the edges there rather than trusting the preview's skew.
- Until there is an ICC profile the master's colour is taken as sRGB;
  renditions are remade once there is one.

# 009 — The master says where the print lies (#9)

**Status:** done

## Why

Masters are not rotated: straightening resamples the pixels, so it belongs
to the rendition. A rendition made from a master alone then needs to know
where the print lies in it. The Side's YAML records the print and region,
but only in bed millimetres and only in the bag. Both are fixed at capture,
so the format statement now puts them in the master's XMP.

## What changes

The master's XMP carries `bs:print` and `bs:region`, each with `bs:angle`
(degrees, story 004's sign), and `bs:centreX`, `bs:centreY`, `bs:width`,
`bs:height` in the master's pixels from its top left, along the print's
own axes.

| Region found | angle | centre, size |
|---|---|---|
| under 0.1° or over 10° | 0.00 | the region square to the bed |
| 0.1° to 10° | the signed angle, to 0.01° | the region turned, to 0.1 px |

The master's origin is the bounds' top left; the detail pass rounds it to
the nearest scan pixel, so the position holds to half a scan pixel.

## Out of scope

Rotating the master (the format statement keeps it unrotated) and
measuring the angle at full resolution (story 003).

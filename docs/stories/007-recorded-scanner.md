# 007 — Recorded scanner: a real print, replayed without hardware (#7)

**Status:** done (code); the capture of a real print waits for the 4990

## Why

`batchtest` serves flat grey rectangles, which is enough for the batch's
logic and nothing like a photograph. Judging the rest of the UI, and the
renditions to come (#3), needs real pixels: paper texture, a skewed edge,
a sepia histogram. One real print, captured once from the 4990 and
replayed from disk, gives that to every session without the scanner.

## What changes

- **`bytestone-scan capture -print <name> [-side front|back]`** records
  one print lying on the bed into `<staging>/fixtures/<name>/<side>/`:
  the preview, a detail scan of the print's region (5 mm of margin) at
  2400 ppi as `scanimage` wrote it, and `capture.yaml`: the scanner, its
  settings, the print as found, the scan's bounds and ppi. The exact
  command lines go in `commands.txt`.
- **`bytestone-scan serve -recorded <name>`** runs the UI on the
  recorded scanner instead of the 4990.
- The recorded scanner's **Preview** is the capture's preview, and
  **Detail** crops the capture to the area asked for and averages it down
  to the ppi asked for; outside the capture is the lid, white.
- Images never go in the repo: the fixtures live under the staging root
  and move to the NAS with the archive (roadmap 10).

## Acceptance

### Capture

| On the bed | Result |
|---|---|
| one print | `preview.png`, `detail.pnm`, `capture.yaml` written for the side |
| none, or more than one | refused, saying how many were found; nothing written |
| the detail pass fails | refused with the scanner's message; no `detail.pnm` left |

| Side | Directory |
|---|---|
| front (default) | `fixtures/<name>/front/` |
| back | `fixtures/<name>/back/`; the print is turned over in place first |

### Replay

| Asked | Served |
|---|---|
| Preview | the capture's preview, 150 ppi |
| Detail of an area within the capture at 600, 1200 or 2400 ppi | that area, each 4×4, 2×2 or 1×1 block of the capture averaged to one sample |
| Detail of an area reaching past the capture | the part within, the rest white |
| Detail at a ppi the capture's does not divide by | refused, naming the capture's ppi |
| Name, Settings | "Recorded: " and the real scanner's, from `capture.yaml` |
| a side not captured | refused, naming the directory |

### Round trip

| | |
|---|---|
| a batch run on the recording of a scanner | the same masters, to the rounding of a pixel, as a batch run on that scanner |

## Out of scope

Backs in the workflow (roadmap 5): the back is captured now so it is there
when they arrive. Several prints per fixture. A 75 ppi preview: the code
previews at 150 ppi, and the recording serves what the batch asks for.

## Decisions

- The preview is captured at 150 ppi, not the 75 the issue first asked
  for: `find` is tuned to 150, and a 75 ppi fixture would test nothing
  the batch runs.
- The capture's ppi is a parameter of the recording (2400 from the CLI),
  so tests can record a small fixture quickly and replay it at 600.
- Detail replays by streaming rows, as the real detail pass does, so an
  830 MB capture is never held in memory.

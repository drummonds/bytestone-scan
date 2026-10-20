# 002 — Run a batch's fronts from a web UI (#2)

**Status:** in progress. Done: looking (work queue, batch page, quality page), the actions (new batch, rescan the preview, confirm, rescan at another ppi, back to the prints, accept, retry) with redo through gobank-workflow's Rewind, one step at a time in the background. Left: splitting and removing regions; moving to gobank-workflow v0.9.0 once released

## Why

The point of the first batches is to find out whether the process is good:
whether the masters are sharp, well exposed and complete. That needs a
person looking at each master, and sending the batch back a step when
something is wrong: moving a print and scanning the preview again, fixing
a region, rescanning at another ppi. A command line makes each of those
awkward. This story puts the batch workflow behind a lofigui UI, adds a
quality check after the detail pass, and lets any step be redone.

## What changes

- `bytestone-scan serve`: a lofigui app on localhost, replacing the
  command line's `batch`, `confirm` and `resume`.
- The batch list and each batch's step history are gobank-workflow's
  `view` pages, mounted in the app.
- A new waiting step, **check quality**, between the detail pass and
  packaging.
- **Redo:** going back to an earlier step clears it and every later
  step, and runs them again. This needs Rewind in gobank-workflow, a story
  of its own there.
- Regions are adjusted at confirm with arrows on their edges, with the
  close-up and preview redrawn; a merged region can be split. No JavaScript.
- The scan runs in the background while the page shows its progress.
  There is one scanner, so one step runs at a time.

## Acceptance

### Steps

| Step | Kind | Page while it is the current step |
|---|---|---|
| describe | run | New batch: description, home person, scan ppi |
| preview | run | Scanning, with the time so far |
| find prints | run | — |
| confirm | wait | Prints: the annotated preview and each region's millimetres |
| detail pass | run | Scanning, with the time so far and the expected time |
| check quality | wait | Quality |
| package | run | Done: the bags made |

### Prints page

| Action | Result |
|---|---|
| an arrow on a region's edge | that edge moves 0.5 mm (single arrow) or 3 mm (double), outwards or inwards; the close-up and preview are redrawn |
| — | an arrow is left out where its move cannot happen: outwards at the bed's edge, inwards below 20 mm; an outward move stops at the bed |
| split a region | divided at the widest strip of background across it, into two numbered regions; refused if none is 5 mm wide |
| remove a region | gone; the rest renumbered |
| rescan the preview | redo from preview |
| confirm | the detail pass starts; refused with no regions |

### Quality page, per photo

| Shows | |
|---|---|
| the master | scaled to fit |
| a 1:1 crop | at full master resolution, wherever the operator clicks on the master |
| histogram | per channel, within the print 3 mm in from its edges (the margin and curled corners show the lid, which clips by design), with the share of samples at 0 and at 65535 |
| measurements | print size in mm, master size in pixels, master ppi, averaging, skew, fill, any warning |

| Action | Result |
|---|---|
| accept | packaging starts |
| rescan at 600, 1200 or 2400 ppi | the batch's scan ppi changed; redo from detail pass |
| back to the prints | redo from confirm |

### Redo

| Redo from | Cleared and run again | Photo ids |
|---|---|---|
| preview | preview onwards | reservations released |
| confirm | confirm onwards | kept (released for regions removed, once regions can be edited) |
| detail pass | detail pass onwards | kept |
| any step, once a bag is complete | refused: a bag is never altered | — |

### Concurrency

| While | A second step asked for | Result |
|---|---|---|
| a step is running | any | refused: "the scanner is busy with batch …" |
| the server restarts mid-step | — | the step is failed by the engine's rules and can be retried |

### Regions

| Rule | Value |
|---|---|
| margin round a print | 0.5 mm: tight by default, just covering detection's ±0.4 mm at 75 ppi; story 001's 3 mm, then 1.5 mm, were too loose on the first real print. The arrows loosen it where wanted |

### Images are labelled

| Page | Image | Caption |
|---|---|---|
| batch | the preview | preview, 75 ppi, with the regions found |
| photo | the master | master, its ppi and size in pixels, shown scaled to fit |
| photo | a crop | 1:1, master pixels, and where |

## Out of scope

Backs (story 3), richer metadata and per-photo homes (story 4),
publishing, master storage, dragging regions with the mouse.

## Decisions

- Region editing by arrow buttons, not dragging, keeps the UI within
  lofigui's HTML-and-HTMX ceiling.
- Redo belongs in the engine, not here: clearing steps by hand would put
  the run's history out of step with its work.

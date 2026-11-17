# 174 — At startup the scanner says which scanners are connected: the FF-680W as well as the 4990 (#174)

## Why

On 2026-11-17 the FF-680W was attached by USB, the 4990 was not, and
`bytestone-scan serve` started with "Epson Perfection 4990 Photo (SANE
epson2)" on its line, as About does. That name is a constant: neither
scanner is looked for until a scan or a feed first runs, so the operator
about to feed letters is told of a scanner that is not there and nothing
of the one that is.

When the first look does come, its failure is kept for the life of the
process: a scanner plugged in after a refused scan is never found
without a restart.

## What changes

- `bytestone-scan serve`: the address line no longer names a scanner. It
  is followed by one line for the bed scanner and one for the feeder,
  each with the SANE device it was found at or "not connected". The
  look runs beside the server, which does not wait for it.
- About: the *Scanner* row becomes *Bed scanner* and *Feeder*, each
  showing the device last found, "not connected", or "not yet looked
  for"; the subtitle names letters and the FF-680W beside prints and the
  4990.
- `internal/epson`, `internal/fastfoto`: a scanner not found is looked
  for again the next time it is wanted; one found is kept, as now.
- README: the startup lines.

## Acceptance

| 4990 (epson2 USB) | FF-680W (epsonds) | Outcome |
|---|---|---|
| connected | USB | startup names both with their devices; About the same |
| absent | USB | startup: bed scanner not connected, feeder at its `epsonds:libusb` device; About the same |
| absent | network only | feeder at its `epsonds:net` device |
| connected | absent | bed scanner at its device, feeder not connected |
| absent | absent | both not connected; the server serves all the same |
| `scanimage` cannot be run | the same | each line says the scanners could not be listed and why; the server serves |
| any | absent at startup, plugged in after, *Scan letters* pressed | the feed runs; About then shows the device |
| absent at startup, plugged in after, a preview asked for | any | the preview runs; About then shows the device |
| absent, a preview refused, then plugged in, a preview asked for | any | the second preview runs |
| `-recorded NAME` | USB | bed scanner line names the recording, no look for a 4990; feeder at its device |
| any | any, About opened before the look ends | "not yet looked for" on the rows still out; the page does not wait |

## Out of scope

- A scanner unplugged and plugged in again after it was found: its USB
  address changes and the kept device is stale until a restart. As now.
- Choosing between scanners, or a `-device` for the feeder: a feed uses
  the feeder and a scan the bed, as now.
- Looking again on a timer, or a status mark on the batch page.
- Anything about letters themselves: sequence, the person they came
  from, sender and receiver are the stories after this one.

## Decisions

None.

## Done when

- [x] tests express the acceptance table and pass
- [x] README updated
- [x] changelog fragment
- [ ] issue closed with a link to the release

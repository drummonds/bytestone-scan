# 117 — Scan letters through the FF-680W sheet feeder: a feed of sheets, both sides, arranged into letters, each archived as an item (#160)

**Status:** built 2026-11-09; deposits need life-weaver story 035 (letter items)
**Roadmap:** none — the format statement's *Letters* section is the spec; story 116 (import a film) is numbered on its own branch, so this is 117

## Why

The first bundle of letters is to be archived. A letter is paper with
two sides a sheet, often several sheets and an envelope; it goes
through the Epson FastFoto FF-680W's document feeder, not onto the
4990's bed, and both sides come in one pass. The scanner knows only the
bed: a preview, prints found and cut, fronts and backs as two passes. A
letter has nothing to find or cut: the sheet is the region, and the
cutting of pages out of a sheet, the turning of cross-written text, the
reading-order PDF and the transcription are editor work later, on
renditions. What is needed now is to feed the sheets, say which make
which letter, keep every side whole, and archive each letter as the
item life-weaver story 035 takes.

## Vocabulary

| Term | Meaning |
|---|---|
| Feed | One load of the sheet feeder within a batch, numbered among its scans as an import is: its description, every side of every sheet as fed, both sides in one pass, and the letters the operator arranges the sheets into |
| Sheet | One piece of paper fed, two sides, kept whole; its role is `sheet` or `envelope` |
| Side | The front or back of a sheet, one master each at 600 ppi, 8 bits per channel, the feeder's limit; a blank side is kept as scanned |
| Arrange | The feed step where the operator says which sheets make which letter, in what order, and each sheet's role; one load is usually one letter, so every sheet starts in letter 1 |
| Letter | An item of kind `letter`: its sheets' sides as masters, `sheet-01-front.tif`, `sheet-01-back.tif`, …, with `letter.yaml`; deposited and receipted as a photo is, with a set of renditions per side under the side's own name |
| Feeder | The FF-680W as the scanner drives it: `scanimage` on the SANE `epsonds` backend, source *ADF Duplex*, colour, 8 bits, 600 ppi, skew correction off, one PNM per side until the feeder is empty |

## Model

A feed is a scan whose bed is the feeder: `Scan.Feed` set, as an
import has `Scan.Page`, so it is numbered with the scans, listed with
them, finished and purged with them. Its workflow is its own type,
`scan_feed`, since its steps share nothing with a bed scan's:

| Step | Does |
|---|---|
| describe | the feed's description is recorded, pre-filled from the batch's title |
| feed | the feeder runs until empty, each side written as an 8-bit TIFF under the feed with a small preview beside it; `fed.yaml` records the scanner, its settings, the ppi and when |
| arrange | waits for the operator to arrange the sheets into letters; then reserves an id for each letter |
| masters | each letter's sides are written as its masters with XMP, and `letter.yaml` beside them, under `masters/<id>/` |
| letters | a letter workflow, `scan_letter`, is started for each |
| check quality | waits for the operator to accept the feed, having looked at each letter's sides and turned any that lie wrong |

A letter's workflow is a photo's without the back wait and the
description: develop (every side's renditions, the whole side fit to
the screen, given its turn), accepted, package (final), deposit,
receipt. The batch waits for feeds as it waits for scans; a letter
counts as a photo does in the batch's outcome, the statistics and the
purge.

## What changes

- `internal/fastfoto`: the feeder, found among `scanimage -L` as the
  `epsonds` USB device, else the network one; `Feed(ctx, dir, ppi)`
  runs `scanimage --batch` into dir and counts the sides; an empty
  feeder is an error that says to load the sheets.
- `internal/batch`: `Env.Feeder`; `NewFeed`; the feed's stages and the
  letter's; `Arrange` (sheets to letters, roles); `TurnSide`; the
  `Letter` record and its XMP; a letter's bag; items, standing, outcome,
  deletion and purge count letters.
- `internal/archive/archivetest`: the rendition check reads the sets
  that are there, as life-weaver 035 does.
- `internal/web`: *Scan letters* on the batch page; the feed page: the
  sheets as fed, front and back side by side, the arrange form (letter
  number and role per sheet) at the arrange wait, the letters listed
  after; the letter page: every side at display size with Turn ↺ ↻ until
  accepted, the record, the archive's answer; *Accept* at the quality
  wait.
- Glossary (Feed, Sheet, Arrange, Letter, Feeder), README, the workflow
  definitions and diagrams.

## Acceptance

The feeder (`internal/fastfoto`, a stand-in `scanimage`):

| Call | Outcome |
|---|---|
| `Feed` with the USB device listed | `scanimage -d epsonds:libusb:… --source "ADF Duplex" --mode Color --depth 8 --resolution 600 --adf-skew=no --format=pnm --batch=<dir>/side-%d.pnm`; the sides counted in order |
| `Feed` with only the network device listed | `-d epsonds:net:…` |
| no epsonds device | an error saying the FF-680W is not connected |
| the feeder empty | an error saying to load the sheets; no sides |

The feed (a fake feeder serving four sides):

| Step | State | Outcome |
|---|---|---|
| feed | four sides served | `sheets/side-1.tif` … `side-4.tif`, 8-bit, 600 ppi; `fed.yaml`; waits at arrange |
| arrange | nothing said | the form offers two sheets, each letter 1, role sheet |
| arrange | sheet 2 an envelope | one letter of two sheets, the second an envelope; one id reserved |
| arrange | sheet 2 in letter 2 | two letters, one sheet each; two ids, in letter order |
| arrange | an odd number of sides | refused: the feed is duplex, sides come in pairs |
| masters | one letter | `masters/<id>/sheet-01-front.tif`, `sheet-01-back.tif`, `sheet-02-front.tif`, `sheet-02-back.tif`, `letter.yaml`; the XMP names the id, the side, the batch, the feed, the scanner, 600 ppi |
| letters | | a `scan_letter` run per letter; each developed: a set per side under `renditions/<id>/<v>/sheet-01-front/` … |
| turn | sheet 1's back turned ↻ before accept | `letter.yaml` says `turn: 90` on that side; its set made again, the display turned |
| turn | after accept | refused |
| accept | | each letter bagged (`data/sheet-NN-*.tif`, `letter.yaml`), deposited, receipted; the batch's outcome counts them held |
| the batch | a feed and a scan | Finish waits for both; the statistics count the feed among the scans and its letters among the photos |

letter.yaml carries `id`, `batch`, `feed`, `description`, `home`,
`provenance`, `credit`, `scanner`, `settings`, `ppi` 600, `depth` 8,
`scanned`, and `sheets`, each `role`, `front` and `back` as `file` and
`turn`.

The web app:

| Page | Shows |
|---|---|
| batch | *Scan letters* beside *Import pages*; a feed listed as *Feed n* among the scans |
| new feed | the description, and a word to load the sheets face down, the first sheet on top, before starting |
| feed, at arrange | each sheet's two sides as thumbnails; a letter number and a role per sheet; *Arrange into letters ›* |
| feed, after | the letters, each linked, with its sheets; *Accept and archive ›* at the quality wait |
| letter | every side at display size in order, *Sheet 1, front* …, Turn ↺ and ↻ until accepted; description, sheets, scanned, provenance, credit; the archive's answer |

## Out of scope

- Correspondents, dates and the people a letter names: claims, a later
  story, once the hand can be transcribed.
- Cutting pages from a sheet, the PDF in reading order, OCR: renditions,
  later.
- Leaving a sheet out (a misfeed) or feeding again: cancel the feed and
  feed afresh; redo of a feed's steps.
- Double-feed detection and the feeder's own crop: the feeder's
  defaults, read back into `fed.yaml`; tested on a real sheet.

## Decisions

A feed is a scan variant, as an import is, so that the batch's listing,
finishing, purging and deletion need no second path; its workflow is a
type of its own because none of its steps is a bed scan's, and the
steps table and the diagrams draw a definition. A letter's workflow is
a type of its own for the same reason, and its page is its own: a
photo's quality page is about one print's edges and levels.

The sides are written as TIFFs at the feed step without XMP and again,
as masters with XMP, once the letters are known: the pixels are
captured once; the id a master carries is not known until the operator
has arranged the sheets.

## Done when

- [x] tests express the acceptance table and pass
- [x] glossary, README, definitions and diagrams updated
- [x] changelog fragment
- [ ] issue closed with a link to the release

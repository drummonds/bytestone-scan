# 177 — A feed on the real FF-680W: its PNM is read, and each side is cut to the sheet (#177)

## Why

On 2026-11-17 the first real sheet, a letter card, was fed through the
FF-680W with the feed's own command line. Two things stand between that
and a letter in the archive.

`scanimage` wrote both sides, 5096 × 9283 at 600 ppi, each beginning

    P6
    # SANE data follows
    5096 9283
    255

`fastfoto`'s reader takes the width straight after `P6` and fails on
the comment: "PNM header: expected integer". The tests' stand-in writes
a header with no comment, which is why they pass. Comments are part of
the PNM format, anywhere in the header before the maxval.

And the side is not the sheet. The feeder scans the full 215.9 mm of its
path, the 117 mm card in the middle of the path's pale grey, and stops
about 10 mm past the paper's end; the driver then fills the rest of the
393.7 mm with pure white. Each side is 142 MB, most of it not the
letter. `--adf-crp`, on or off, changes nothing in SANE 1.2.1, and `-x`
narrows the scan only to a width given before the sheet is fed, one for
the whole load. So the scanner cuts the side to the sheet itself, at
capture, before the master is written.

## What changes

- The format statement, *Letters*: each side is cut across to the sheet
  and 3 mm of the path either side; the driver's white fill is left off;
  the feeder's pictures come as JPEG, quality 90, decoded into the
  master. (Drafted in bytestoneblog, 2026-11-17.)
- `internal/fastfoto`: a side's PNM header is read with its comment
  lines passed over. Each side is handed over cut: the rows of white
  fill after the last scanned row left off, and the columns cut to the
  sheet and 3 mm either side. The path is told from the sheet by being
  flat down the side's length and the path's grey; the cut runs from the
  outermost column that is not path, 1 mm at each edge of the path left
  out of the search, so what is uncertain errs wide and the sheet is
  never cut into.
- `internal/fastfoto`: the settings recorded with a master name the cut
  and the JPEG transfer.
- `internal/fastfoto/testdata/scanimage`: the stand-in writes the
  header as the real `scanimage` does, comment line included.
- README, under letters: what the master of a side holds.

## Acceptance

The reader:

| The side's PNM header | Outcome |
|---|---|
| `P6`, `# SANE data follows`, width height, maxval 255 (as the FF-680W through `scanimage`) | the side is read, its samples as before |
| no comment (as the stand-in wrote until now) | the side is read, as before |
| a comment after the width and height | the side is read |
| a comment, then a maxval other than 255 | refused as not an 8-bit colour scan, as before |
| the pixels short of width × height | refused, naming the row, as before |

The cut, at 600 ppi:

| The side as fed | Across | Down |
|---|---|---|
| a coloured sheet in the middle of the path, grey either side, white fill below | the sheet and 3 mm of path either side | the scanned rows; the fill left off |
| a white sheet on the grey path | the sheet and 3 mm either side | the scanned rows |
| a sheet with a shadow along its edges | the shadow is within the cut, 3 mm of path beyond it | the scanned rows |
| a sheet nearer than 3 mm to one edge of the path | to the path's edge on that side, 3 mm on the other | the scanned rows |
| a sheet the full width of the path, no grey | the full width | the scanned rows |
| a sheet fed askew | every column the sheet reaches and 3 mm either side | the scanned rows |
| a sheet as long as the path, no fill | as its width gives | every row |
| a blank side | cut as any other, by its paper against the path | the scanned rows |
| a streak down the path outside the sheet, further than 1 mm from the path's edge | from the streak to the sheet's far side and 3 mm: wide, the sheet whole | the scanned rows |
| white columns within 1 mm of the path's edge (as the real feed gave) | not taken for sheet: the sheet and 3 mm | the scanned rows |
| nothing but path: no sheet found | the full width | the scanned rows |
| the front and the back of one sheet | each cut on its own; they need not be the same width | each its own |

| Recorded with the master | Outcome |
|---|---|
| the feeder's settings | as before, and that each side is cut to the sheet and 3 mm, and that the transfer is JPEG quality 90 |

## Out of scope

- A surround at the top: the feeder starts at the sheet's leading edge
  and gives none. The foot keeps what the feeder gives past the paper's
  end, about 10 mm; it is not trimmed to 3.
- Straightening a sheet fed askew, cutting pages out, turning: all
  renditions, as the statement has it.
- The JPEG quality: `scanimage` has no option for it; 90 is the driver's.
- The cut's offsets recorded per side. The master's size is in its TIFF.
- A feed begun without the 4990, the person a letter is from, sender
  and receiver, pages, OCR: later stories.

## Decisions

Cut in software at capture, not by `-x`: a load may mix sheet sizes
(user, 2026-11-17). No ADR.

## Done when

- [x] tests express the acceptance tables and pass
- [x] README updated
- [x] changelog fragment
- [ ] a real coloured sheet and a real white sheet fed through the
      built scanner, the masters looked at
- [ ] issue closed with a link to the release

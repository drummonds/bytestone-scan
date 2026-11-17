# 177 — A feed on the real FF-680W fails: scanimage's PNM carries a comment line the reader refuses (#177)

## Why

On 2026-11-17 the first real sheet, a letter card, was fed through the
FF-680W with the feed's own command line. `scanimage` wrote both sides,
5096 × 9283 at 600 ppi, each beginning

    P6
    # SANE data follows
    5096 9283
    255

`fastfoto`'s reader takes the width straight after `P6` and fails on
the comment: "PNM header: expected integer". So *Scan letters* cannot
work on the scanner as it stands; the tests' stand-in writes a header
with no comment, which is why they pass. Comments are part of the PNM
format, anywhere in the header before the maxval.

## What changes

- `internal/fastfoto`: a side's PNM header is read with its comment
  lines passed over.
- `internal/fastfoto/testdata/scanimage`: the stand-in writes the
  header as the real `scanimage` does, comment line included.

## Acceptance

| The side's PNM header | Outcome |
|---|---|
| `P6`, `# SANE data follows`, width height, maxval 255 (as the FF-680W through `scanimage`) | the side is read, its samples as before |
| no comment (as the stand-in wrote until now) | the side is read, as before |
| a comment after the width and height | the side is read |
| a comment, then a maxval other than 255 | refused as not an 8-bit colour scan, as before |
| the pixels short of width × height | refused, naming the row, as before |

## Out of scope

- The sheet's surround: the real feed returns the whole 215.9 × 393.7 mm
  feed path with the sheet at its top, `--adf-crp` on or not. Cutting
  the capture to the sheet is a rule for the statement first, and its
  own story.
- The 4990's reader (`internal/master`), which this story does not
  touch.

## Decisions

None.

## Done when

- [ ] tests express the acceptance table and pass
- [ ] changelog fragment
- [ ] issue closed with a link to the release

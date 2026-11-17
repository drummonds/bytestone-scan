# 181 — A blank side is left out of a letter: the operator marks it at arrange, and the record says it was blank (#181)

## Why

The feeder scans both sides of every sheet, and most sheets of most
letters are written on one side. As the statement stands every side is
kept, "blank backs included": a four-sheet letter is eight masters, four
of them empty paper at 30 to 60 MB each, and its page and any PDF or
transcription made later carry four empty sides between the written
ones.

The statement's reason for keeping them was that a blank master "says
the side was looked at". The record can say that without the picture.

## What changes

- The format statement, *Letters*: a side the archivist finds blank is
  left out of the item, and `letter.yaml` records it as blank; a sheet
  blank on both sides is left out whole. This replaces "A blank side is
  kept as scanned". Statement first, then the code.
- Arrange (`internal/batch`, `internal/web`): each side has a *blank:
  leave out* mark beside its picture, off by default. Nothing is marked
  for the operator.
- `letter.yaml`: a side left out has `blank: true` and no `file`. A
  sheet with both sides left out is not among the letter's sheets, and
  the sheets after it are numbered without a gap.
- The letter's page: a side left out is said to be blank, with no
  picture, no turn and no renditions.
- README, under letters.

## Acceptance

| At arrange the operator | Outcome |
|---|---|
| marks nothing | every side kept, as before |
| marks the back of a sheet blank | the letter holds `sheet-NN-front.tif` and no `sheet-NN-back.tif`; `letter.yaml` has that sheet's back as `blank: true` with no file; no renditions are made for it |
| marks the front blank, the back kept | `sheet-NN-back.tif` kept, no front; the front recorded blank |
| marks both sides of a sheet, the letter having other sheets | the sheet is not in the letter; the sheets after it take the next numbers, no gap; nothing of it in `letter.yaml` |
| marks every side of every sheet of a letter | arrange is refused, saying a letter needs a side; nothing is reserved |
| marks a side of an envelope | as a sheet's: the role makes no difference |
| arranges again before accepting, the mark taken off | the side is back in the letter |

| Then | Outcome |
|---|---|
| the letter's page | a side left out is named and said to be blank; no picture, no turn offered |
| the letter accepted | the bag holds the kept sides' masters and `letter.yaml` only; a set of renditions per kept side |
| deposited with the archive | receipted; the archive's page shows the kept sides in order (life-weaver passes over a side with no file already) |
| the feed's page | says how many sides were fed and how many left out |
| the feed's work files | still hold every side as fed until the batch is purged, as now |

## Out of scope

- The scanner judging a side blank, or proposing the mark. Faint pencil,
  an embossed heading and show-through are the hard cases, and what a
  blank side of this collection looks like is not yet known; the
  operator sees every side at arrange. Its own idea once there are
  feeds to learn from.
- Leaving out a side that is not blank: a duplicate, a wrong sheet.
  Sheets already go to one letter or another; a sheet that belongs to
  none is another story.
- Taking a blank side out of a letter already deposited.
- A blank side's master kept but hidden.

## Decisions

Reverses the 2026-11-09 decision that every side is kept, blank backs
included (user, 2026-11-17). The operator decides, not the scanner. No
ADR.

## Done when

- [ ] the statement's *Letters* section changed
- [ ] tests express the acceptance tables and pass
- [ ] README updated
- [ ] changelog fragment
- [ ] a letter with a side left out deposited with the UAT archive and
      looked at on its site
- [ ] issue closed with a link to the release

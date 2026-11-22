# 202 — A letter's sides are cut into pages: the operator says where a side folds and the order its pages are read in (#202)

## Why

A letter is kept as whole sheets, and a sheet is not a page. The first
real letter fed (2m, 22 Nov 2026) is one sheet folded in half: its front
carries page 1 and a blank page 4, its back pages 2 and 3, and both lie
on their side as fed. The letter's page shows two sideways pictures in
sheet order, which is not how the letter reads.

Reading it, by eye or by a model, and showing text beside picture both
need the letter as pages: upright, one at a time, in reading order. A
spike on 2m (`spike/pages`, 22 Nov 2026) showed the cut is mechanical
once the fold and the turn are known, and that neither can be found by
the machine: Tesseract's orientation detection gave confidences of 1.37
and 0.07 on the two sides.

This is experimental. The layout and the page pictures live in the
scanner until the shape settles; life-weaver will have to know how to
show them later.

## What changes

- Glossary: *Letter page*, one face of a letter as it is read, cut from
  a side. (*Page* alone is already a file received from elsewhere.)
- The letter's page (`internal/web`), before the feed is accepted: beside
  each side's Turn buttons, *Folded in half* (off by default) and a
  number for each of its pages. A side not folded is one page; a side
  folded in half is two, left and right of the middle of the side as
  turned. A page may be marked blank in place of a number.
- The two sides of a sheet are coupled, since the feeder sees the back
  flipped about the way the sheet travels. Turning a side turns the
  sheet's other side the opposite way by as much, and marking a side
  folded, or taking the fold off, does the same to the other. *This side
  only* beside the Turn buttons turns one side alone, for a back written
  the other way up. Until now a turn moved its own side only.
- `letter.yaml` (`internal/batch`): each side lists its pages, left to
  right as turned, each with its number or `blank: true`. The numbers of
  a letter run from 1 with no gap and none twice; that is its reading
  order.
- Renditions (`internal/rendition`): a picture of each numbered page,
  cut from the master at the middle of the turned side, upright, in its
  side's rendition set as `page-NN.jpg`. The master is untouched and the
  side's own renditions are made as now. Recipe version 3.
- The letter's page gains *Pages*: the numbered pages in reading order,
  each picture linked to its side.
- The format statement, *Letters*: a sentence saying the layout of pages
  is recorded in `letter.yaml` and the pages are renditions. No rule
  changes.
- README, under letters.

## Acceptance

| On the letter's page, before accepting, the operator | Outcome |
|---|---|
| does nothing | each side is one page, numbered in sheet order, front before back; *Pages* shows them so |
| turns a sheet's front a quarter turn clockwise | the front is turned clockwise and the back a quarter turn anticlockwise; both `turn`s recorded |
| turns a sheet's back half a turn | the front is turned half a turn too |
| turns a side with *This side only* | that side alone is turned; the other is as it was, and later turns of either still move both by as much |
| turns a side whose other side was left out as blank | that side alone is turned |
| marks a side folded in half | both sides of that sheet have two pages; the letter's pages are numbered again in sheet order, front before back, left before right |
| on one folded sheet, numbers the front 4 and 1 and the back 2 and 3 | *Pages* shows 1, 2, 3, 4 in that order: front right, back left, back right, front left |
| marks page 4 blank | *Pages* shows 1, 2, 3; `letter.yaml` has the front's left page as `blank: true`; no picture is made of it |
| turns a side after folding it | its pages are cut left and right of the side as now turned, each upright |
| takes the fold off a side | both sides of that sheet are one page again, and the letter's pages are numbered again in sheet order |
| marks folded a side whose other side was left out as blank | that side has two pages; the side left out has none |
| gives two pages the same number, or leaves a gap | refused, saying which number is twice or missing; nothing is changed |
| marks every page of the letter blank | refused: a letter needs a page |
| looks at a side left out as blank (story 181) | no fold or numbers offered; it has no pages |
| looks at an envelope's side | as a sheet's |

| Then | Outcome |
|---|---|
| the feed accepted | the bag's `letter.yaml` carries each side's pages; the masters are as scanned; the bag holds no page pictures |
| the letter's page after accepting | *Pages* still shown; fold and numbers no longer offered |
| deposited with the archive | receipted; the archive's page shows the sides as before (life-weaver ignores the pages; untried) |
| a letter accepted before this story | each side is shown as one page in sheet order; its `letter.yaml` is not rewritten |

## Out of scope

- The reading: the text of each page as djot, typed or from a model, and
  the view of text beside picture. The next story, built on these pages.
- Proposing the fold, the turn or the order by machine.
- Finding where the fold really lies. The cut is at the middle; a sheet
  folded off-centre shows a sliver of its neighbour.
- Any other layout: three folds, a page written across itself, a side
  written both ways up.
- Laying out a letter already deposited (2m among them). Feed it again
  to try this.
- life-weaver showing pages, and a PDF in reading order.

## Decisions

- The layout is a record on the side in `letter.yaml`, not a property of
  the master: it says how to cut, and the cut is a rendition (user,
  22 Nov 2026: experimental, lives in the scanner for now).
- A sheet's sides are coupled in turn and fold (user, 22 Nov 2026).
  Which way is up and which side is the outside are still the
  operator's: they are the four ways a folded sheet can be fed, and
  only reading it tells them apart.
- The term is *Letter page*, since *Page* is taken. Mine; say if you
  would rather rename the received *Page*.
- No ADR.

## Done when

- [ ] the statement's *Letters* section changed
- [ ] tests express the acceptance tables and pass
- [ ] glossary and README updated
- [ ] changelog fragment
- [ ] a folded letter laid out, deposited with the UAT archive and
      looked at
- [ ] issue closed with a link to the release

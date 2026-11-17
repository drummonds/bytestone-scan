# 185 — A scan or a feed is not begun when its scanner is not connected, and a batch page says what letters are fed at (#185)

## Why

On 2026-11-17, with the FF-680W attached and the 4990 not, two batches
(20261117-6 and -8) were begun from *New batch* by its first button,
*Scan the preview*. Each recorded a batch and a bed scan that failed at
once: "the Epson 4990 is not connected". A failed scan keeps *Finish
scanning* waiting, so each batch sat in the Queue until deleted. Since
story 174 the scanner can say whether the 4990 is there before anything
is recorded.

The batch that did hold a letter (20261117-9) says "Scanned at 2400
ppi", the bed's setting, though its one feed was at 600; and its
workflow is "Archive photographs", for a batch that holds a letter.

## What changes

- `internal/web`: before a batch is recorded for a bed scan, or a bed
  scan for a batch, the bed scanner is looked for; before a batch is
  recorded for letters, or a feed for a batch, the feeder is. Not found,
  nothing is recorded: the form comes back with what was typed and the
  reason. A scanner that is not looked for (a recording) is begun with
  as before.
- The batch page: *Scanned at* becomes *Prints scanned at* (the batch's
  ppi) and *Letters fed at* (600 ppi, 8 bits).
- `internal/batch`: the batch's workflow is named *Archive a batch*.
- README, where it says what New batch does.

## Acceptance

| Asked for | Its scanner | Outcome |
|---|---|---|
| New batch, *Scan the preview* | the 4990 connected | the batch and its first scan begun, as before |
| New batch, *Scan the preview* | the 4990 not connected | no batch recorded; the form back with what was typed and "the Epson 4990 is not connected"; nothing in the Queue |
| New batch, *Scan letters instead* | the FF-680W connected | the batch recorded; the feed's form, as before |
| New batch, *Scan letters instead* | the FF-680W not connected | no batch recorded; the form back with what was typed and "the Epson FF-680W is not connected" |
| New batch, *Import pages instead* or *Import a film instead* | neither connected | the batch recorded, as before: neither is needed |
| a batch's *Do one more scan*, submitted | the 4990 not connected | no scan recorded; the form back with the description typed and the reason; the batch finishes as it would have |
| a batch's *Scan letters*, submitted | the FF-680W not connected | no feed recorded; the form back with the description typed and the reason |
| any of these, refused, the scanner then plugged in, asked again | found on the second look | begun |
| any of these | the scanners cannot be listed | refused with that reason; nothing recorded |
| New batch, *Scan the preview* | a recording (`-recorded`): not looked for | begun, as before |

| Page | Outcome |
|---|---|
| a batch's page | *Prints scanned at* the batch's ppi; *Letters fed at* 600 ppi, 8 bits |
| a batch's page, the Queue and its filter | the batch's workflow is *Archive a batch* |

## Out of scope

- A scanner unplugged after it was found: the device kept is stale and
  the scan fails as now (story 174's out of scope).
- Which button New batch puts first, or hiding the one whose scanner is
  absent: what was last found may be out of date, and the look on
  pressing is what is trusted.
- New batch's wording about prints and its *Scan at* choice for a batch
  of letters only.
- A failed scan keeping *Finish scanning* waiting.

## Decisions

None.

## Done when

- [ ] tests express the acceptance tables and pass
- [ ] README updated
- [ ] changelog fragment
- [ ] issue closed with a link to the release

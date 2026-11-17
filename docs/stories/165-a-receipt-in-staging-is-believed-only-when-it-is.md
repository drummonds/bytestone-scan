# 165 — A receipt in staging is believed only when it is this bag's: a photo is never taken as deposited on another archive's receipt (#165)

## Why

On 2026-11-16 the scanner was first run against the hosted UAT service
(life-weaver story 188). That tree did not yet hold the items deposited
on 5 Nov through the service then run on the laptop, so it gave out
`1r` again, for a photo of another person. Staging still held the 5 Nov
`bags/1r`, its receipt and `renditions/1r`. The batch page showed the
new photo as through, on the old receipt.

Had the scan gone on, packaging would have found a complete bag under
the id and removed the new masters, the deposit step would have found a
receipt and sent nothing, and only the receipt step would have failed,
the masters gone. Staging is kept per instance *name*, and two trees may
bear one name, or one tree may lose items, so the name does not make an
id staging holds safe to take.

## What changes

- `internal/batch`: an id taken from the archive for a print, a page
  item, a letter or a film is refused when staging holds `bags/<id>`,
  `bags/<id>.deposit.yaml` or `renditions/<id>`. The step that asked
  (confirm, arrange, a film's describe) fails and says which files; the item is given no
  id, so nothing is scanned, bagged or deposited under it.
- README, under staging: what the refusal means and what to do.

## Acceptance

| The archive gives out | Staging holds under that id | Outcome |
|---|---|---|
| a print's id | nothing | the print has the id; the scan goes on (as before) |
| a print's id | a bag | confirm fails, naming the item and `bags/<id>`; the print has no id; no detail pass; the bag untouched |
| a print's id | a receipt alone | the same, naming `bags/<id>.deposit.yaml` |
| a print's id | renditions alone | the same, naming `renditions/<id>` |
| a print's id, retried once the files are moved away | nothing under the next id | the print has the next id; the scan goes on |
| a letter's id | a bag | arrange fails, naming the item and the bag; the letter has no id |
| a film's id | renditions | the film's describe fails, naming the item and `renditions/<id>`; the film has no id |
| a page item's id | any of the three | as a print's: one `reserve` serves all four |

## Out of scope

- Giving the refused id back. The archive has no call to release a
  reservation; the id stays reserved and is a gap in the series.
- Checking a receipt's checksums against the bag where the receipt is
  read (#165's other suggestion). Refusing the id comes before the
  master is scanned, and is the one that keeps packaging from removing
  it; with the id refused no later step meets a receipt not its own.
- Why the archive gave the id out again, and UAT's trees agreeing:
  life-weaver's.
- Ids staging holds that the archive has *not* given out again: old
  batches whose items the archive lacks are `verify`'s to report.

## Decisions

Refuse at reservation, not at deposit; none recorded as an ADR.

## Done when

- [x] tests express the acceptance table and pass
- [x] README updated
- [x] changelog fragment
- [ ] issue closed with a link to the release

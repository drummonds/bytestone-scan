# 105 — Progress dots on the batch and scan pages, and a retry for a failed photo (#135)

**Status:** built 2026-11-02

## Why

Batch 20261027-1 has two imports whose photos the archive refused at
deposit. The batch page says so in a long sentence per scan; the scan
page says "Done: 0 photos held by the archive; 1 not yet deposited",
hides why, and offers nothing to do about it, though a tick of the scan
re-attempts every failed photo. gobank-workflow 0.15.0 added
`view.Views.Progress`, a run as a row of coloured dots, for exactly this
page. Follows #132.

## What changes

- Batch page, *Steps*: a line of dots for the batch's own run, and under
  it one row per scan: the scan's label linking to its page, then a dot
  per photo coloured by the photo's run and linking to the photo; a scan
  with no photos yet is one dot for its own run. Each row is titled with
  the scan's Standing.
- Scan page: the scan's own run as a line of dots under the title, with
  its photo row.
- Scan page, scan through: when a photo's run has failed or been
  cancelled, the notice is that photo's Standing in red, with a *Retry*
  button that ticks the scan (the existing retry action), instead of the
  outcome sentence.

## Acceptance

| Page | State | Outcome |
|---|---|---|
| batch | run started | a `dots` row linking to the batch's instance, one dot per batch step titled `<step>: <status>` |
| batch | scan with photos | a row titled with the scan's Standing, a dot per photo linking to `photos/<id>`, titled `photo <id>: <status>` |
| batch | scan at confirm, no photos | a row of one dot, titled `waiting at confirm` |
| batch | photo failed at deposit | that photo's dot is red (`#f14668`) |
| scan | run started | a `dots` row of the scan's steps linking to its instance, then the photo row |
| scan | through, every photo held | "Done: … held by the archive", as before |
| scan | through, a photo failed | red notice `photo <id> failed at deposit: …` and a Retry button posting to the scan's retry action |
| scan | retry posted | the failed photo's run is attempted again |

## Out of scope

- A per-photo retry: the scan's tick re-runs every photo of it, which is
  the unit the worker takes.
- Dots on the batches list and the photo page.

## Decisions

none

## Done when

- [x] tests express the acceptance table and pass
- [x] changelog fragment
- [ ] issue closed with a link to the release

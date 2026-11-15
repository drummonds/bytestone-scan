# 115 — The scanner says what staging holds as JSON at /status, for life-weaver's status page (#156)

**Status:** built 2026-11-06

## Why

life-weaver's status page (its stories 029 and 030) shows the service,
the slots, the archive trees on the NAS and the sites, but not the
scanner: how much its staging holds, on which instance, and how the
photos there stand with the archive. The scanner already counts all of
that on its *Statistics* page (#117), as HTML for a person. life-weaver
cannot read the scanner's staging itself without the scanner's batch
code, and the two modules already depend one way (the scanner imports
life-weaver's `pid` and `person`). So the scanner says the same figures
as JSON, and life-weaver asks for them (its story 031).

## What changes

- `internal/web`: `GET /status` answers `Status` as JSON: `version` (the
  scanner's build), `instance` (the archive instance it runs for, or
  `error` when the archive is not answering), `staging` (the root),
  `batches` by state (`open`, `finished`, `closed`, `purged`), `scans`,
  `photos` (`held`, `pending`, `rejected`), `usage` (each entry at the
  staging root with its `name` and `bytes`) and `total` bytes. The
  figures are `batch.Statistics`'s, the ones the *Statistics* page
  shows; the runs in the workflow database stay on the page.
- README names the call, under the web app.

## Acceptance

| Call | Scanner | Outcome |
|---|---|---|
| `GET /status` | one batch, one scan, two photos held | `200 application/json`; `version`; `instance` example, scratch, slot 1; `staging` the root; `batches.open: 1`; `scans: 1`; `photos.held: 2`; `usage` names `batches`, `bags`, `renditions`; `total` positive; no `error` |
| `GET /status` | the archive down | `200`; `error` says so; the staging figures still there |
| `GET /status` | staging unreadable | `200`; `error` says what failed |

## Out of scope

life-weaver's side: asking the scanner ports and showing the answer is
its story 031. The runs by workflow and status: they are the database's,
not staging's.

## Decisions

A JSON answer the archive service asks for, rather than life-weaver
reading staging: the figures stay the scanner's, computed once by its
own code, and no module cycle. `/status` for the name life-weaver uses
for the same thing.

## Done when

- [x] tests express the acceptance table and pass
- [x] README updated
- [x] changelog fragment
- [ ] issue closed with a link to the release

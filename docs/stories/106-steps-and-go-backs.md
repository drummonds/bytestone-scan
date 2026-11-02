# 106 — Steps, errors and go-backs on every page, from the workflow definitions (#139)

**Status:** proposed 2026-11-02

## Why

Photo 16 of batch 20261027-1 failed at deposit. Its page says

> Not deposited: failed at deposit: deposit: item 16: depositing item 16:
> refused for its renditions: data/20261027081257928-000.jpg is missing or
> not this bag's; redeveloped at recipe 2, to deposit again

Four layers name the same step and item before the cause, and "to
deposit again" names no actor: nothing deposits again until the operator
presses Retry on the scan page, and the photo page has no Retry. The
standing reads the run's error, which the engine prefixes with the stage
name, where the step's own error is on record without it; the scanner's
deposit and the archive client each prefix the item too.

The runs are keyed by the batch id, the scan key and the photo id, and a
child run names its parent, so the database already chains batch to scan
to photo. The engine's instance page shows that chain as Parent and
Children; the scanner's own pages do not, and the photo page shows no
steps at all. Batch to scan reads well; scan to photo and photo back up
do not.

Redo sends a scan back to any step and Rewind refuses past a final one,
but the pages offer the go-backs only in certain states and under
unrelated names: "Rescan the preview", "Back to the prints", "Rescan at",
"Rescan the backs". The definition says the operator can send the scan
back to any step until its photos are archived; nothing on the pages
shows it. There are only five spots to go back to, the steps Redo clears
markers for: preview, confirm, fronts, backs preview and backs. Follows
#135.

## What changes

- Scan page and photo page: a *Steps* table as the batch page has, drawn
  from `batch.Definitions`: step, status, error, action. The batch page's
  table gains the error and action columns.
- Every page: a Parent line above the steps (the batch for a scan, the
  scan for a photo) and the children below (the batch's scans, the scan's
  photos), each with its own step and status, as the engine's instance
  page has them.
- Action column: *Back to this step* for a completed step the scan can go
  back to; *Retry* for a failed step, the existing retry action; nothing
  else. The waiting step's choices stay in the state bar, unchanged.
- Photo page, Archive section: a *Retry* that posts to the scan's retry
  action and returns to the photo.
- Failure text says the cause and the next step only. `Standing` reads
  the step's error, not the run's; `depositItem` and the archive client
  stop naming the item; `Refusal` for renditions says what the archive
  wants: "the archive wants a rendition of data/….jpg and the bag has
  none; the renditions were remade at recipe 2, Retry deposits again".
- The existing go-back buttons in the state bar stay; they are the same
  Redo and read the same step names.

## Acceptance

| Page | Step | Status | Condition | Action |
|---|---|---|---|---|
| scan | preview, confirm, fronts, backs preview or backs | completed | no photo of the scan packaged | *Back to <step>*, posting the scan's Redo from that step |
| scan | any | completed | a photo packaged | none; the row says archived and final |
| scan | any other | completed | | none |
| scan | any | failed | | *Retry*, posting the scan's retry; the row shows the step's error |
| scan | any | waiting | | none in the table; the state bar's choices as now |
| photo | any | failed | | *Retry*, posting the scan's retry, returning to the photo; the row shows the step's error |
| photo | package or later | completed | | none; the row says final |
| batch | scans | failed | | none; the scan rows say which scan |
| any | any | not run | | none |

| Page | Element | Outcome |
|---|---|---|
| scan | Parent | links to its batch, with the batch's step and status |
| scan | Children | one row per photo: its id linking to its page, its step and status |
| photo | Parent | links to its scan, with the scan's step and status |
| batch | Children | one row per scan, as the Scans and imports table; a scan's step and status |
| photo | Archive, run failed | "The archive refused the deposit: …" with the cause from the step's error and a *Retry* |
| photo, scan, batch | failure text | names the step once and the item not at all: `refused for its renditions` appears once, no `deposit: item 16: depositing item 16:` |

## Out of scope

- A per-photo retry: the scan's tick re-runs every photo of it, as #135
  decided.
- Describing a photo again before it is packaged.
- Batch, scan and photo tables in the database: the bag is the format and
  the run keys are the ids; the pages show the chain the runs already
  have.
- Knowing that a retry will fail the same way until the archive is fixed
  (life-weaver#110): the page says what the archive said and what Retry
  does.

## Decisions

- The dots row stays as the glance; the Steps table carries the controls.
- The go-back spots are the five steps Redo clears markers for; not every
  step is a sensible target, so the table does not offer them all.

## Done when

- [ ] tests express the acceptance tables and pass
- [ ] changelog fragment
- [ ] issue closed with a link to the release

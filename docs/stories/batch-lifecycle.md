# Batch lifecycle: a batch of loads, deposited, closed, purged

**Status:** proposed (2026-10-24, revised the same day after discussion);
to be split into issues once the tables below are agreed

## Why

Today a batch is one load of the bed. In practice one sitting empties a
shoebox: one home person, one provenance, one credit, one scan ppi, many
loads of the bed. Then the photos have to reach the bytestone record,
which life-weaver keeps, and the staging copies can go. The tool stops at
the bag; everything after it is by hand, and nothing says when staging may
be cleared.

## Where the scanner's job ends

The scanner captures and describes; life-weaver keeps. Rights, publishing,
the copies on the NAS, the site and offsite, and fixity are life-weaver's,
not the scanner's. The scanner supplies what it knows (home person,
provenance, credit, the load's description, the capture record) and asks
life-weaver one question: *do you hold this photo, checksums matching?*
Once the answer is yes for every photo of the batch, the batch's work
files are redundant, whatever the photo's rights turn out to be.

That question has to be an API of life-weaver's, not a look at
i.bytestone.uk: a photo whose rights are undetermined is never on the
site, and a stalled deploy must not hold a batch open.

## Vocabulary

| Term | Meaning |
|---|---|
| Batch | A sitting of scanning for one home person: its title, provenance, credit, scan ppi, the albums its photos join, and its loads. Open from describe until every photo is receipted, then closed, then purged |
| Load | One load of the scanner bed within a batch: its description, preview, prints (one or many), detail pass (fronts, later backs), masters and renditions. What Batch means today |
| Deposit | The batch step that hands each photo to life-weaver: its bag, its item record for its home person, and the albums it joins |
| Receipt | life-weaver's answer that it holds a photo, the bag's checksums matching; asked for over its API |
| Work files | What a batch keeps under staging to do its job: previews, prints, masters, renditions, bags, the runs' records |
| Close | The batch step reached when every photo has a receipt: its work files are marked for deletion |
| Purge | Deleting a closed batch's work files, 30 days after close or sooner when staging is short of space |

Not terms: *scan* for a load (scan already means the preview, the detail
pass and the ppi); *job* (Batch covers it); *publish*, *store*, *fixity*
(life-weaver's words, not the scanner's).

The format statement changes first: "The scanning resolution is chosen per
batch" stays true; the XMP records "the batch and the load"; Packaging
gains a paragraph saying that the scanner's staging holds a photo only
until life-weaver receipts it, and is not one of the archive's copies.

## What moves where

| Was on the batch | Now on |
|---|---|
| description | the load, pre-filled from the batch's title; photo.yaml's description. A load's prints share it: per-print descriptions at capture are out of scope |
| home person, provenance, credit, scan ppi | the batch, unchanged |
| preview, find prints, confirm, detail pass, develop, check quality, package | the load |
| — | batch: title, albums, deposit, close |

photo.yaml and the master's XMP carry both ids: `batch: 20261024-1`,
`load: 2`. An item's id stays its own; the batch and load say how it was
captured, not where it belongs.

## Lifecycle

```d2
direction: right
Open: {
  describe -> loads: "any number; each a load workflow"
  loads -> loads: "next load"
}
Open -> Deposit: "operator: scanning finished;\nevery load packaged"
Deposit -> Closed: "every photo receipted"
Closed -> Purged: "30 days, or staging short of space"
```

## Workflows

Three levels, each a definition of its own, each instance a child of the
one above. The engine has no child runs yet (its own comparison table
marks the gap), so the engine grows first; see the engine changes below.

| Definition | Key | Steps |
|---|---|---|
| `batch` | batch id | describe → **loads** (sub-process: a `load` per load of the bed, until the operator ends scanning) → close (waits until every photo of every load is receipted) |
| `load` | `<batch id>/<n>` | describe → preview → find prints → confirm → detail pass → **photos** (sub-process: a `photo` per print, started here) → check quality (accept the load's masters, or send the load back a step) |
| `photo` | photo id | develop → accepted (waits for its load's check quality) → package → deposit → receipt (waits for life-weaver) |

A sub-process step starts its children; it does not wait for them. Waiting
is a later step's business: the photo parks at *accepted* until its load
passes check quality, and the batch parks at *close* until every photo is
receipted. Both are the engine's existing park-and-poll, so a load's
quality check and life-weaver's receipt need no new primitive. Sending a
load back to the detail pass rewinds its photos' develop with it; a photo
once packaged is final, as today.

Purge is a sweep over closed batches at startup and daily, not a step: a
run parked for a month is a run nobody can see the end of.

### Pictures

Each definition's page (`workflows/definitions/<type>`) shows its own
picture: its steps in a row, a sub-process drawn as one collapsed box with
BPMN's `+` marker, linking to the child definition's page. One picture
answers one question, so a definition that does not fit across a screen
is split: a run of steps becomes a sub-process with its own page. The
three above are 3, 7 and 5 boxes wide.

The engine's `diagram.Definition` already writes the d2; the view already
serves `<type>.svg` from a `Diagrams` filesystem the host supplies. The
scanner supplies none today, so the page has no picture. A `diagrams` task
renders the scanner's definitions with the d2 CLI into
`internal/web/diagrams/`, embedded and committed, so `go install` needs no
d2. The view's definitions list shows only the scanner's own workflows:
today it is handed the engine's whole registry, which the library seeds
with *end of day* and *release*.

### Engine changes (gobank-workflow)

| Change | For |
|---|---|
| `parent_run_id` on runs, a store query for a run's children and descendants by status | the hierarchy; the batch's close rule |
| a way to start a run under a parent, and a `Step` that names a child definition and its multiplicity | sub-process steps |
| instance page lists a run's children with their status; definition page draws a sub-process as a collapsed box linking to the child | pictures and navigation |
| the library's `Definitions` starts empty; *end of day* moves to gobank, *release* to task-plus | a host sees only what it runs |
| `writing-workflows.md`: a definition fits across one screen, else split | the pictures rule |

### Close

| Photo | Counts toward close? |
|---|---|
| receipted: life-weaver holds the bag, checksums matching | yes |
| deposited, no receipt yet | no |
| not yet deposited | no |

A batch closes when every photo counts. Rights do not appear: a photo life
weaver holds is safe to let go of whether or not it will ever be shown.

### Purge

| Condition | Purge |
|---|---|
| closed 30 days ago or more | at the next sweep |
| staging free space under the threshold | oldest closed batch first, until above it |
| open batch | never |

The 30 days are a grace period: a mistake found after hand-over can still
be recovered from staging. The threshold is one constant until there is a
reason for it not to be. Purge deletes every work file of the batch; there
is nothing in staging that is not regenerable or already held.

## Deposit: the life-weaver contract

Under active development on life-weaver's side. What the scanner needs:

| Call | Scanner uses it for |
|---|---|
| people and their albums | the describe form: home person, albums to join |
| deposit a photo | the bag, the item record (type photo, home person, provenance, credit, description), the albums to join |
| receipt for a photo id | the Close rule: held, with the bag's manifest hash |

Open: how the bag's bytes travel, an upload to the API or a path life
weaver can read. The tables above do not depend on the answer.

## Stories

One at a time, in this order unless backs are wanted sooner:

| Story | Delivers | Depends on |
|---|---|---|
| 0. Definitions page (#24) | only the scanner's workflows listed; each with its picture | — |
| E. Child runs (gobank-workflow#35, #36) | the engine changes above | — |
| A. Batch of loads | `batch`, `load` and `photo` definitions as above; batch page lists loads, New load; both ids in photo.yaml and XMP; existing staging batches become one-load batches by hand | 0, E |
| B. Backs (roadmap 5) | A load scans fronts then backs | A |
| C. Deposit | albums chosen at describe; each photo deposited; receipts polled | A, life-weaver's API |
| D. Close and purge | the close rule, the sweep, the retention table | C |

Roadmap 6 (summary sheet and label) becomes per batch, after A. Roadmap 9
(publish) and 10 (NAS, fixity, offsite) leave this roadmap for
life-weaver's: the scanner's part of them is Deposit.

## Out of scope

- Rights, publishing, the archive's copies and fixity: life-weaver.
- Per-print descriptions at capture: a load's prints share its
  description; small prints are scanned many to a load.
- Re-opening a closed batch: a new batch instead.

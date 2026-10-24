# Batch lifecycle: a batch of loads, recorded, published, stored, closed

**Status:** proposed (2026-10-24); to be split into issues once the
vocabulary below is agreed

## Why

Today a batch is one load of the bed. In practice one sitting empties a
shoebox: one home person, one provenance, one credit, one scan ppi, many
loads of the bed, then the photos go into the person's record and onto
the site, the bags go to the NAS, and the staging copies can go. The tool
stops at the bag. Everything after it is by hand, and nothing says when
staging may be cleared.

## Vocabulary

| Term | Meaning |
|---|---|
| Batch | A sitting of scanning for one home person: its title, provenance, credit, scan ppi, the albums its photos join, and its loads. Open from describe until its photos are recorded, published and stored, then closed, then purged |
| Load | One load of the scanner bed within a batch: its description, preview, prints, detail pass (fronts, later backs), masters and renditions. What Batch means today |
| Record | The batch step that writes each photo as an item of its home person, and into the batch's albums, in life-weaver: the bytestone record |
| Publish | The batch step that waits until i.bytestone.uk serves every photo whose rights allow it, checksums matching |
| Store | The batch step that waits until the NAS holds every bag, checksums matching |
| Work files | What a load keeps under staging to do its job: preview, prints, renditions, the run's records; and the staging copy of each bag |
| Close | The batch step that marks its work files for deletion once every photo is recorded, published and stored |
| Purge | Deleting a closed batch's work files, a month after close or sooner when staging is short of space |

Not terms: *scan* for a load (scan already means the preview, the detail
pass and the ppi); *job* (Batch covers it).

The format statement changes first: "The scanning resolution is chosen per
batch" stays true; the XMP records "the batch and the load"; Packaging
gains a paragraph on what staging holds and when it is cleared.

## What moves where

| Was on the batch | Now on |
|---|---|
| description | the load (pre-filled from the batch's title); photo.yaml's description |
| home person, provenance, credit, scan ppi | the batch, unchanged |
| preview, prints, confirm, detail pass, develop, check quality, package | the load |
| — | batch: title, albums, record, publish, store, close |

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
Open -> Record: "operator: scanning finished;\nevery load packaged"
Record -> Publish
Publish -> Store
Store -> Closed: "every photo recorded,\npublished where allowed, stored"
Closed -> Purged: "30 days, or staging short of space"
```

Two workflow types in gobank-workflow, which has no nested runs: `batch`
keyed by batch id, `load` keyed by `<batch id>/<n>`. The batch's *scan*
step parks while loads are open and is done when the operator ends
scanning and every load is packaged. Publish and Store park and are
polled, like confirm and check quality today. Purge is a sweep over closed
batches at startup and daily, not a step: a run parked for a month is a
run nobody can see the end of.

### When a batch may close

| Photo's rights | On the NAS, checksums match | On the site, checksums match | Recorded in life-weaver | Close? |
|---|---|---|---|---|
| allow publishing | yes | yes | yes | yes |
| allow publishing | yes | no | — | no: not public yet |
| allow publishing | no | — | — | no: not stored |
| do not allow, or undetermined | yes | not applicable | yes | yes |
| any | — | — | no | no |

### What purge deletes

| Work file | Deleted at purge? |
|---|---|
| preview, prints, renditions in staging, run records | yes |
| staging copy of a bag whose photo has two verified copies (NAS and site) | yes |
| staging copy of a bag whose photo has one verified copy (NAS only: rights do not allow publishing) | no, until the offsite copy exists; the statement's open question |

So a batch of unpublishable photos still closes; its bags stay in staging
as the second copy. Staging is then part of the archive's copies for those
photos, and the fixity check must include it.

### Retention

| Condition | Purge |
|---|---|
| closed 30 days ago or more | at the next sweep |
| staging free space under the threshold | oldest closed batch first, until above it |
| open batch | never |

The threshold is one constant until there is a reason for it not to be.

## Record: writing into life-weaver

Recommended: write item records and album entries into life-weaver's data
directory in life-weaver's format, the same way the people register is
read today; committing that repository is life-weaver's business, not the
scanner's. The alternative, a life-weaver command the scanner calls, is the
cleaner boundary and can replace the direct write later without changing
this story's tables. Needs life-weaver#19 to fix the item record format
and where album membership is kept.

Albums are chosen at describe from the home person's record, so the
operator says where the photos are going before scanning starts; Record
uses the choice. A batch may have no albums.

## Stories

One at a time, in this order unless backs are wanted sooner:

| Story | Delivers | Depends on |
|---|---|---|
| A. Batch of loads | Batch with title and loads; load workflow; batch page lists loads, New load; ids in photo.yaml and XMP; existing staging batches become one-load batches by hand | — |
| B. Backs (roadmap 5) | A load scans fronts then backs | A |
| C. Record (roadmap 9, part) | albums chosen at describe; items and album membership written to life-weaver | A, life-weaver#19 |
| D. Store (roadmap 10, part) | bags copied to the NAS and verified against the manifests | A |
| E. Publish (roadmap 9, rest) | the site's copies verified; the step waits for life-weaver's deploy | C |
| F. Close and purge | the close rule, the sweep, the retention table | D, E |

Roadmap 6 (summary sheet and label) becomes per batch, after A.

## Out of scope

- The offsite copy: where it lives is the statement's open question; until
  it exists, unpublishable bags stay in staging.
- Fixity on the NAS and the site as a scheduled job (roadmap 10): Store and
  Publish verify once, at the step.
- Re-opening a closed batch: a new batch instead.

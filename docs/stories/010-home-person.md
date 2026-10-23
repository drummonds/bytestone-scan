# 010 — New batch chooses the home person by name; a glossary (#10)

**Status:** done

## Why

Starting a batch means typing the home person's id, which has to be
looked up elsewhere. And the vocabulary has moved on: a person has a
collection of items of several kinds (photos, letters, notebooks, …), of
which this tool archives one, and albums are arrangements of items made
on the site, not something a capture decides. The words need settling in
one place the operator can see before the next stories build on them.

The archive model this follows is life-weaver#19: one id series for every
item, type as a field, the home person as a field, albums as curated lists
of item ids kept with the person's record.

## What changes

- **Glossary** on the About page, from one source that README and the
  docs link to. README's Vocabulary table goes.
- **New batch** chooses the home person from the register, listed by
  name with id, instead of typing the id.
- **`internal/bag` becomes `internal/archive`**: the archive of items,
  flat by id. `bag.Bag` becomes `archive.Photo`, the one photo being
  written; `Reserve`, `Create`, `Complete`, `Release` keep their names.
  The staging directory `bags/` becomes `archive/`. Bags already under
  staging are moved by hand, once; there is no migration code.
- The format on disk is unchanged: each photo is still a BagIt bag.

## Acceptance

### Glossary

| Term | Meaning |
|---|---|
| Item | One thing in a person's collection: a photo, letter, notebook, …; has its own id, from one series for every type |
| Photo | An item that is a photographic print; its archived record is kept as a BagIt bag (RFC 8493) holding its masters and photo.yaml |
| Home | The person whose collection an item belongs to |
| Album | A titled, ordered arrangement of items, kept with a person's record; an item may be in many albums |
| Batch, Print, Side, Region, Bounds, Preview, Detail pass, Scan ppi, Master ppi, Master, Rendition | as README's table today |

Bag is not a term: it is the format a Photo is kept in.

### New batch: home person

| Register | Shown |
|---|---|
| people | one option each, `Name (id)`, sorted by name |
| a record with no name | left out, with a warning naming its file |
| no people | the form refused, naming the register directory |

| Input | Result |
|---|---|
| a person chosen | batch recorded with that person, as now |
| no person chosen | refused |
| an id not in the register (form tampered or record removed) | refused, as now |

## Out of scope

- Albums in the scanner: adding a batch's photos to an album is part of
  the batch's Save step, written to life-weaver, after renditions (#3).
- Duplicate detection against photos already archived.
- One id allocator shared by every tool that archives items: this tool is
  the first, so its archive is where the series starts.
- Searching people at scale (life-weaver#10).

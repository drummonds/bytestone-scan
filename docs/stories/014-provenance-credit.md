# 014 — Batch provenance and credit, copied into each photo (#14)

**Status:** done

## Why

A photo's record is written once and never altered, and the batch's
working files go at Save. So whatever is known about a print's origin at
capture, and the terms it may be shown under, has to be in photo.yaml
before packaging or it is lost. Both are the same for every print in a
load, so the operator enters them once, for the batch.

Only capture facts belong here. Dates, places and the people shown are
claims (roadmap: transcribe backs into claims) and stay out.

## What changes

- **New batch** asks for two more things: **provenance**, where the
  prints came from ("Aunt Jean's shoebox, envelope marked 1950s"), and
  **credit**, the rights line life-weaver's album entries carry
  (`© Drummond Family, CC BY 4.0`).
- Both are pre-filled from the most recent batch: a shoebox is many loads
  of the bed.
- Each photo.yaml carries the batch's provenance and credit. The master's
  XMP carries the credit as `dc:rights`, so the TIFF states its terms by
  itself.
- The batch page and the quality page show both.

## Acceptance

### New batch

| Input | Result |
|---|---|
| credit given | recorded, whitespace collapsed |
| credit blank | refused: a photo cannot be shown without its terms |
| provenance given | recorded, whitespace collapsed |
| provenance blank | recorded as none; left out of photo.yaml |

| Earlier batches | The form's provenance and credit |
|---|---|
| none | blank |
| some | the most recently created batch's |
| refused | what was typed, kept |

### Each photo

| Where | Carries |
|---|---|
| photo.yaml | `provenance` (omitted when none), `credit` |
| front.tif XMP | `dc:rights`: the credit |
| bag-info.txt | unchanged |

### Shown

| Page | Rows |
|---|---|
| batch | Provenance, Credit |
| photo | Provenance, Credit |

## Out of scope

Per-photo provenance or credit; a register of credits to choose from;
dates, places and people (claims); changing a batch's provenance or
credit after its photos are packaged (never: photo.yaml is final).

## Decisions

- Credit is required, provenance is not: a print of unknown origin can
  still be archived, but nothing can be shown without its terms.
- Pre-filling from the latest batch, rather than a configured default,
  keeps a family's name out of the code and still means typing each
  line once per shoebox.

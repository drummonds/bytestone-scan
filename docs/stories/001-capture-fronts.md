# 001 — Capture the fronts of a batch (#1)

**Status:** proposed

## Why

Scanning is slow at archival resolution, about two minutes per print at
2400 ppi, so the bed is loaded with several prints at a time. Each needs its
own master, archived as a bag on its own, and filed under the person it
belongs to. This story builds the capture end to end for fronts. Its first
real run is the SS Bonita print at 2400 ppi.

## What changes

- A `bytestone-scan` CLI that runs one batch as a gobank-workflow
  instance keyed by batch id, with state in pglike so a batch survives a
  restart.
- **Describe:** the operator gives the batch a description, a home person
  and a scan ppi. The person is checked against the life-weaver register.
- **Preview:** a 75 ppi bed scan. **Find prints:** regions found on it,
  each with a margin. **Confirm:** an annotated preview PNG, accepted or
  adjusted on the command line.
- **Detail pass:** one scan of the band covering every region at the scan
  ppi. Rows are averaged down as they stream and cut into one master per
  region; full resolution is never held in memory.
- **Masters:** 16-bit RGB baseline TIFF, Deflate with the horizontal
  predictor, exact resolution, capture facts embedded as XMP. Go's
  `x/image/tiff` cannot write this, so the writer is our own.
- **Package:** one bag per photo under the staging root.

## Acceptance

### Describe

| Description | Person | Scan ppi | Result |
|---|---|---|---|
| given | in the register | 600, 1200 or 2400 | batch recorded |
| empty | — | — | refused: a batch needs a description |
| given | not in the register | — | refused, naming the id |
| given | in the register | anything else | refused, listing 600, 1200, 2400 |

### Master ppi and averaging

| Print, short × long side | Scan ppi | Master ppi | Averaging |
|---|---|---|---|
| at least 4 × 6 in | 600 | 600 | none |
| at least 4 × 6 in | 1200 | 600 | 2 × 2 |
| at least 4 × 6 in | 2400 | 600 | 4 × 4 |
| smaller than 4 × 6 in | 600 | 600 | none, with a warning that 1200 was wanted |
| smaller than 4 × 6 in | 1200 | 1200 | none |
| smaller than 4 × 6 in | 2400 | 1200 | 2 × 2 |

Averaging is the mean of each block, rounded to the nearest count.

### Find prints, from the preview

| Preview shows | Regions |
|---|---|
| one print | one, the print plus 3 mm each side, clipped to the bed |
| several prints at least 5 mm apart | one each, numbered top to bottom then left to right |
| two prints closer than 5 mm | one region for both; the operator splits it at confirm |
| a mark smaller than 20 × 20 mm | ignored |
| no print | none; the batch stops at confirm |

The lid is white and so are many print borders, so detection is weakest
there; black card behind the prints makes it reliable.

### Detail pass

| Regions | Scanned |
|---|---|
| any | once: from the top of the highest region to the bottom of the lowest, left of the leftmost to right of the rightmost |
| scan fails part-way | no master written; retrying the batch scans again |

### Master

| Property | Value |
|---|---|
| Format | TIFF 6.0 baseline, little-endian, one image |
| Samples | RGB, 16 bits each, no alpha |
| Compression | Deflate (8) with horizontal predictor (2) |
| Resolution | master ppi exactly, as a rational |
| Embedded XMP | photo id, side, batch id, capture time, scanner, scan ppi, averaging, scanner settings |
| Pixels | identical to the averaged scan: reading the file back gives the same values |

### Bag

| Path | Content |
|---|---|
| `<photo-id>/bagit.txt` | BagIt 1.0, UTF-8 |
| `<photo-id>/bag-info.txt` | batch id, description, bagging date |
| `<photo-id>/data/front.tif` | the master |
| `<photo-id>/data/photo.yaml` | id, batch, description, home, region on the bed, scan settings |
| `<photo-id>/manifest-sha256.txt` | every file under `data/` |
| `<photo-id>/tagmanifest-sha256.txt` | the tag files |

Photo ids are numbers written in lower-case Crockford base32, in their own
series: the next is one more than the highest bag in the staging root.

### Workflow

| Step | Kind | Re-run after a crash |
|---|---|---|
| describe | run | kept |
| preview | run | scans again |
| find prints | run | finds again from the stored preview |
| confirm | wait | until the operator accepts |
| detail pass | run | scans again; masters are written to a temporary name and renamed when complete |
| package | run | skips a photo whose bag is already complete |

## Out of scope

Backs, the batch summary, the scan button, a web UI, renditions,
publishing, and moving bags to the NAS. All are on the roadmap.

## Decisions

- Two passes, not one per print: scan time follows the carriage's travel,
  so a single detail pass over the band costs little more than the tallest
  print alone.
- Averaging, not the scanner's own reduction: the 4990's 600 ppi mode is
  noisier than averaging 1200 ppi 2 × 2 (statement, Noise).
- Colour correction off and sharpness 0 on every pass (statement,
  Capture).
- Our own TIFF writer: `x/image/tiff` writes 16-bit only with an alpha
  channel, applies the predictor only to LZW, and cannot embed XMP or ICC.

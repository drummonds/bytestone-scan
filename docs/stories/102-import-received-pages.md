# 102 — Import received pages: files captured elsewhere, each page an item, each print on it a photo cut from the page (#102)

**Status:** agreed 2026-10-27; waits on the format statement's *Copies
received* section and life-weaver#72 (page items)

## Why

Not every print reaches the scanner. Three came today as scan-to-email
PDFs from an office copier: 300 ppi, one 8-bit colour JPEG and two 1-bit
halftones, each on an A4 page with a white margin round the print. The
capture was made elsewhere; what is left to do is cut the prints out,
keep the page as the evidence, and describe the photos. The scanner has
no way to take them.

## Vocabulary

| Term | Meaning |
|---|---|
| Import | One bringing-in of received files within a batch, as a Scan is one placing on the bed: its description, its pages, the prints found on each, masters, and the photos archived from them |
| Page | A received file: an image of prints captured by someone else's device, kept unaltered as an item of its own, with an id from the series, homed with the batch's person; its bag holds the file as sent and page.yaml |
| Received side | A side whose master is cut from a page, not scanned here: its ppi and depth are the page's, and its record names the page and where on it the print lay |
| Not received | What a photo's record says of a back that was not sent: not the same as blank |

## Model

An import is a scan whose bed is the page. The preview is the page at
150 ppi; prints are found on it as on a bed scan; the operator confirms
or adjusts regions in the editor already built; the fronts pass cuts each
region from the page, with its margin, instead of driving the scanner.
The photo workflow after that is unchanged: develop, accept, describe,
package, deposit, receipt. The page is deposited before its photos, so
each photo's `cut_from` names an item the archive holds.

The file-serving scanner is the recorded scanner's shape: a
`batch.Scanner` whose Preview is the page scaled to 150 ppi and whose
Detail is the page's pixels over the region at the page's own ppi.

## What changes

- **Batch page** gains *Import pages* beside *One more scan*: a
  description, one or more image files (JPEG, PNG, TIFF), and for the
  pages the sender, the date received and the device if known.
- **An import** is listed with the batch's scans, numbered in the same
  sequence, and leads through the same steps: prints found, confirm,
  check quality, describe, accept. No backs step: the backs were not
  received.
- **Masters** are cut at the depth and resolution received. The glossary's
  scan ppi and master ppi lines say so; `plan.MasterFor` takes the page's
  ppi with no averaging.
- **photo.yaml** carries, under the front, `cut_from` (the page's id) and
  the region on the page; `back_received: false` in place of
  `back_blank`. The quality page says *copy received from page <id>*,
  with the page's format and ppi, where it names the scanner for a scan.
- **XMP** names the page and the region on it where it names the bed and
  the region on it.
- **page.yaml and the page's bag**: the file as sent, unaltered, and the
  record: sender, received, device, format, declared ppi, and the photos
  cut from it. Bagged and deposited by the scanner; receipted like a photo.

## Not in this slice

Pulling the image out of a PDF or an email (today's files were extracted
by hand with `pdfimages`); a back sent as a second file; `supersedes`
when the print is later scanned; perceptual dedupe.

## Acceptance

### Import pages

| Input | Result |
|---|---|
| no file | refused: nothing to import |
| a file that is not JPEG, PNG or TIFF | refused, naming the file and its type |
| a file with no resolution recorded | refused: the ppi must be given on the form |
| ppi given on the form | used for every page in the import, over the file's own |
| sender blank | refused: a page must say who sent it |
| received date blank | today |
| description blank | the batch's title |
| one file | one page, one import |
| several files | one import, one page each, in the order given |

### Each page

| Page | Result |
|---|---|
| 8-bit JPEG, 300 ppi | master 8-bit RGB, 300 ppi, no averaging, no warning |
| 1-bit halftone | master 8-bit grey as RGB, at the page's ppi |
| 16-bit TIFF | master 16-bit, at the page's ppi |
| holds one print | one photo |
| holds several prints | one photo each, found as on a bed |
| no print found | the import waits at confirm with no prints; the page is still deposited |

### Prints and regions

| Case | Result |
|---|---|
| print found | region with the usual margin, clipped to the page |
| operator adjusts, splits, removes | as for a scan |
| region reaches the page's edge | bounds clipped; no warning: the page is the whole of what was received |

### The photo's record

| Where | Carries |
|---|---|
| photo.yaml front | `cut_from`: page id; `print`, `region`, `bounds` on the page in mm at the page's ppi; `scan_ppi` and `master_ppi` the page's; `averaging` 1; `scanner` the page's device or *unknown* |
| photo.yaml | `back_received: false`; no `back`, no `back_blank` |
| front.tif XMP | the page id and the region on it, in place of the bed's |
| bag-info.txt | the batch and the import's number, as a scan's |

### The page's record

| Where | Carries |
|---|---|
| page bag `data/` | the file as sent, its name kept; `page.yaml` |
| page.yaml | `id`, `home`, `batch`, `import`, `sender`, `received`, `device` (omitted when unknown), `format`, `ppi`, `photos`: the ids cut from it |

### Order of deposit

| State | Result |
|---|---|
| page not yet receipted | its photos wait at deposit |
| page rejected by the archive as a duplicate | its photos are cut from the item the archive names instead |
| page receipted | its photos deposit as a scan's do |

### Shown

| Page | Rows |
|---|---|
| batch | each import among the scans, by number, with its page count |
| import | the pages, each with its prints found, as a scan's preview |
| photo | *Copy received*: page id, sender, received, device, format, ppi; *Back*: not received |

# 102 — Import received pages: files captured elsewhere, each page an item, each print on it a photo cut from the page (#102)

**Status:** built 2026-10-27 on life-weaver v0.14.0 (#72, page items);
the format statement's *Copies received* section is the spec

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
| Page | A received file: an image of prints captured by someone else's device, kept unaltered. Holding one print, it is kept in that photo's bag: one id. Holding several, it is an item of its own, with an id from the series, homed with the batch's person; its bag holds the file as sent and page.yaml |
| Received side | A side whose master is cut from a page, not scanned here: its ppi and depth are the page's, and its record names the page and where on it the print lay |
| Not received | What a photo's record says of a back that was not sent: not the same as blank |

## Model

An import is a scan whose bed is the page: one page, one import,
numbered among the batch's scans. The preview is the page at 150 ppi;
prints are found on it as on a bed scan; the operator confirms or adjusts
regions in the editor already built; the fronts pass cuts each region
from the page, with its margin, instead of driving the scanner. The
photo workflow after that is unchanged: develop, accept, describe,
package, deposit, receipt.

Whether the page becomes an item of its own is decided at confirm, when
the prints are counted. One print: the photo's own bag keeps the file as
sent beside the master cut from it, and there is one id. Several: a page
id is reserved after the photos', and once the import is accepted the
page is bagged with page.yaml, deposited and receipted before the photos
are packaged, so each photo's `cut_from` names an item the archive holds.
A page the archive holds already is the one the photos are cut from: their
records and masters are rewritten before packaging.

The file-serving scanner is the recorded scanner's shape: a
`batch.Scanner` whose Preview is the page scaled to 150 ppi and whose
Detail is the page's pixels over the region at the page's own ppi.

## What changes

- **Batch page** gains *Import pages* beside *Do one more scan*, and
  New batch an *Import pages instead* button: a description, one or
  more image files (JPEG, PNG, TIFF), the sender, the date received, the
  device if known, and the ppi when the files do not say.
- **Each file is an import**, listed with the batch's scans, numbered in
  the same sequence, and leads through the same steps: prints found,
  confirm, check quality, describe, accept. No backs step: the backs were
  not received. No rescanning: the page is what it is.
- **Masters** are cut at the depth and resolution received. The glossary's
  scan ppi and master ppi lines say so; `plan.MasterFor` takes the page's
  ppi with no averaging.
- **photo.yaml** carries, under the front, `received` (the page's own
  record: file, format, ppi, depth, sender, received, device), `depth`,
  and `cut_from` when the page is an item; `back_not_received: true`.
  The quality page's *Copy received* row says so, and *Back: not
  received*.
- **XMP** carries the same: sender, received, file, format, ppi, depth,
  device, and `cutFrom`.
- **page.yaml and the page's bag**: the file as sent, unaltered, and the
  record: id, batch, import, description, home, provenance, credit,
  file, format, ppi, depth, sender, received, device, and the photos cut
  from it. Its renditions are the page fit to the screen, untouched.

## Tried on the three pages received

The colour JPEG: one print found, 157 × 109 mm, fill 0.98. The two
halftone pages, 300 ppi given on the form since pdfimages wrote 72 into
the PNGs: the group photograph, which fills its page, is found whole;
the ship's print splits at its white sky into two regions flagged for
review, to be mended in the region editor as a print on a white lid is.

## Not in this slice

A *whole page is the print* shortcut for a page the finder splits.

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
| one file | one import |
| several files | one import each, numbered in the order given; the first runs, the rest follow in turn |

### Each page

| Page | Result |
|---|---|
| 8-bit JPEG, 300 ppi | master 8-bit RGB, 300 ppi, no averaging, no warning |
| 1-bit halftone | master 8-bit grey as RGB, at the page's ppi |
| 16-bit TIFF | master 16-bit, at the page's ppi |
| holds one print | one photo, one id: the file as sent in its bag |
| holds several prints | one photo each, found as on a bed, and a page item, reserved after the photos' ids |
| no print found | the import waits at confirm with no prints, as a scan does |

### Prints and regions

| Case | Result |
|---|---|
| print found | region with the usual margin, clipped to the page |
| operator adjusts, splits, removes | as for a scan |
| region reaches the page's edge | bounds clipped; no warning: the page is the whole of what was received |

### The photo's record

| Where | Carries |
|---|---|
| photo.yaml front | `received`: the page's record; `depth`; `cut_from`: the page id when the page is an item; `print`, `region`, `bounds` on the page in mm; `scan_ppi` and `master_ppi` the page's; `averaging` 1; `scanner` the page's device or *unknown device*; no `warning` |
| photo.yaml | `back_not_received: true`; no `back`, no `back_blank` |
| photo bag, one print | the file as sent, by its name, beside front.tif and photo.yaml |
| front.tif XMP | sender, received, file, format, ppi, depth, device, `cutFrom`; the region as on a bed |
| bag-info.txt | the batch and the import's number, as a scan's |

### The page's record

| Where | Carries |
|---|---|
| page bag `data/` | the file as sent, its name kept; `page.yaml` |
| page.yaml | `id`, `batch`, `import`, `description`, `home`, `provenance`, `credit`, `file`, `format`, `ppi`, `depth`, `sender`, `received`, `device` (omitted when unknown), `photos`: the ids cut from it |
| renditions | the page fit to the screen, recipe naming the file's checksum |

### Order of deposit

| State | Result |
|---|---|
| page not yet deposited | its photos wait at *page held*, before packaging |
| page rejected by the archive as a duplicate | its photos' records and masters name the item the archive holds, then package and deposit |
| page held | its photos package and deposit as a scan's do |
| a photo bag, or the page's, complete | nothing before packaging can be redone, as for a scan |

### Shown

| Page | Rows |
|---|---|
| batch | each import among the scans as *Import n*, with its prints count |
| import | Page (file, format, ppi, depth), Sender, Received, Device in place of *Scanned at*; the page with its regions in place of the preview; *Confirm and cut*, no rescanning, no backs; Back: not received |
| photo | *Copy received*: sender, received, file, format, ppi, depth, device, and the page item cut from; *Back*: not received |

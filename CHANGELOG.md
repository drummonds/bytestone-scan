# Changelog

## [Unreleased]

## [0.15.0] - 2026-10-25

 - The scan page shows the preview, and the backs preview, cut to the rectangle covering every region with 10 mm round it once the prints are confirmed, so the photos and the quality check are not below a page of blank bed; the whole bed is shown until confirm, when a region may be dragged anywhere on it. The stored preview.png is untouched; Finding prints is no longer fooled by the lid's shading: the background is a field, its level estimated per row and per column of the bed from the bed outside the prints, so a shaded band of bed is not taken for a print; a pixel is print when it differs from the background at its own place by over 14 levels, and the mask is opened by 1 mm first, so a hot pixel or a dust grain joins nothing; A caption for each photo: the describe form takes it first, above the date, one line saying what the picture shows in the archivist's own words ("Air Raid Wardens"); kept in photo.yaml as `caption` beside the scan's `description`, carried into the deposit, and shown on the photo page under the picture as its title with the description beneath. Not a claim; blank means no caption, and "Nothing known" keeps it; A fresh batch form starts at 2400 ppi, so fronts are scanned at 2400 and averaged 4 × 4 down to the 600 ppi archival TIFF (2 × 2 to 1200 for prints under 6 × 4 inches) unless the operator picks otherwise; a form returned with an error keeps what was picked

### Added

- A caption for each photo: the describe form takes it first, above the date, one line saying what the picture shows in the archivist's own words ("Air Raid Wardens"); kept in photo.yaml as `caption` beside the scan's `description`, carried into the deposit, and shown on the photo page under the picture as its title with the description beneath. Not a claim; blank means no caption, and "Nothing known" keeps it (#66)

### Changed

- The scan page shows the preview, and the backs preview, cut to the rectangle covering every region with 10 mm round it once the prints are confirmed, so the photos and the quality check are not below a page of blank bed; the whole bed is shown until confirm, when a region may be dragged anywhere on it. The stored preview.png is untouched (#64)
- A fresh batch form starts at 2400 ppi, so fronts are scanned at 2400 and averaged 4 × 4 down to the 600 ppi archival TIFF (2 × 2 to 1200 for prints under 6 × 4 inches) unless the operator picks otherwise; a form returned with an error keeps what was picked (#67)

### Fixed

- Finding prints is no longer fooled by the lid's shading: the background is a field, its level estimated per row and per column of the bed from the bed outside the prints, so a shaded band of bed is not taken for a print; a pixel is print when it differs from the background at its own place by over 14 levels, and the mask is opened by 1 mm first, so a hot pixel or a dust grain joins nothing (#65)

## [0.14.0] - 2026-10-24

 - Delete a batch that will never close: a Delete button on the batch page, confirmed on its own page, cancels the batch's, its scans' and its photos' runs and removes its directory, bags and renditions from staging; refused while the archive holds a photo of the batch (finish scanning and it closes and purges), and not offered once closed. A deleted batch's id is not given out again. The new Batches page (the navbar's Queue is the work queue) lists every batch and where it stands, then the stale data no readable batch claims: batch directories in a layout this build does not read, and id directories under `bags/` and `renditions/` with no batch behind them and no answer from the archive; each deletable the same way. Nothing the archive holds is touched; Load becomes Scan: one placing of prints on the bed is a scan, in the glossary, the format statement, the screens ("Scan 1 of batch X", "Do one more scan", a Scans table on the batch page) and the code: the type `Scan`, pages at `batches/<id>/scans/<n>`, the staging directory `scans/`, the workflow type `scan` (was `scan_load`) with its stage `scans` on the batch workflow, `scan:` in photo.yaml, `bs:scan` in the master's XMP and `Bytestone-Scan` in bag-info.txt. Nothing is migrated: `scan_load` runs in the workflow database and `loads/` directories in staging are not read, since every batch so far is a test

### Added

- Delete a batch that will never close: a Delete button on the batch page, confirmed on its own page, cancels the batch's, its scans' and its photos' runs and removes its directory, bags and renditions from staging; refused while the archive holds a photo of the batch (finish scanning and it closes and purges), and not offered once closed. A deleted batch's id is not given out again. The new Batches page (the navbar's Queue is the work queue) lists every batch and where it stands, then the stale data no readable batch claims: batch directories in a layout this build does not read, and id directories under `bags/` and `renditions/` with no batch behind them and no answer from the archive; each deletable the same way. Nothing the archive holds is touched (#54)

### Changed

- Load becomes Scan: one placing of prints on the bed is a scan, in the glossary, the format statement, the screens ("Scan 1 of batch X", "Do one more scan", a Scans table on the batch page) and the code: the type `Scan`, pages at `batches/<id>/scans/<n>`, the staging directory `scans/`, the workflow type `scan` (was `scan_load`) with its stage `scans` on the batch workflow, `scan:` in photo.yaml, `bs:scan` in the master's XMP and `Bytestone-Scan` in bag-info.txt. Nothing is migrated: `scan_load` runs in the workflow database and `loads/` directories in staging are not read, since every batch so far is a test (#56)

## [0.13.0] - 2026-10-24

 - Deposit carries the renditions (story #55, life-weaver#39): the tar put to `PUT /items/{id}` holds `bag/` and `renditions/<recipe>/` (the front's display, thumb, tiny and recipe; the back's under `back/`), developed first if they are missing; a deposit the archive refuses for its renditions (`400`, status `renditions`) is redeveloped at the current recipe and deposited again on the next tick. Lands with life-weaver#39: a service without it refuses the new tar as a bad bag; A closed batch's notice counts its photos: how many the archive holds and how many it rejected as duplicates, each of those linked; a finished batch's notice the same, with how many are not yet deposited

### Changed

- Deposit carries the renditions (story #55, life-weaver#39): the tar put to `PUT /items/{id}` holds `bag/` and `renditions/<recipe>/` (the front's display, thumb, tiny and recipe; the back's under `back/`), developed first if they are missing; a deposit the archive refuses for its renditions (`400`, status `renditions`) is redeveloped at the current recipe and deposited again on the next tick. Lands with life-weaver#39: a service without it refuses the new tar as a bad bag (#55)
- A closed batch's notice counts its photos: how many the archive holds and how many it rejected as duplicates, each of those linked; a finished batch's notice the same, with how many are not yet deposited (#57)

## [0.12.0] - 2026-10-24

 - Backs (story B of the batch-lifecycle epic, roadmap 5): after the fronts the load waits for the prints to be turned over in place, previews the bed to see where each back lies, flags a back whose size differs from its front’s or that cannot be found, and scans the fronts’ regions again at 300 ppi, 8 bits, as `back.tif` in the bag. Each photo is now a workflow of its own under its load (`scan_photo`: develop front → awaiting back → develop back → accepted → package); develop and package leave the load. The load page shows the backs preview and asks for the prints to be turned, or for the backs to be skipped when they are blank (then the bag has no `back.tif`); the photo page shows the back; the backs can be rescanned. Masters at 8 bits per channel; fakes and the recorded scanner serve the side turned to the glass; Describe each photo (story #40 of the batch-lifecycle epic): on its page, while the load is checked, the operator says what is known of a photo — the date taken (an ISO 8601 prefix, with an approximate flag), the photographer and what the back says — each kept in photo.yaml as a claim with its source (the back or front of the print, or a relative’s word) and how it was read, as the format statement has it; nothing known is an answer. The photo workflow gains a `describe` step after `accepted`: a photo not yet described waits there, and the load page lists them; once archived the record is shown and fixed; Deposit and receipt (story #41 of the batch-lifecycle epic, life-weaver#33): photo ids come from the archive service (`POST /people/{home}/items`), not a local series; once bagged each photo is deposited (`PUT /items/{id}`, a tar of the bag) and its receipt read back and checked against the bag, checksum for checksum; a bag the archive refuses as a duplicate ends the photo, rejected, shown on its page and the load’s; a service that is down fails the step until retried. Staging keeps bags under `bags/` with the archive’s answer beside each. The service client is `internal/archive`, the bag writer `internal/bag`; tests use `archivetest`, an in-memory service that follows life-weaver’s deposit table; Close and purge (story #42, the last of the batch-lifecycle epic): a batch closes when scanning is finished and every photo is held by the archive or rejected by it; its work files (loads, held photos’ bags and renditions) are purged 30 days after it closes, or sooner, oldest closed first, while staging has under 10 GiB free, at startup and daily; never an open batch’s, and a rejected photo’s bag is kept. The batch page says when it closed and when it was purged

### Added

- Backs (story B of the batch-lifecycle epic, roadmap 5): after the fronts the load waits for the prints to be turned over in place, previews the bed to see where each back lies, flags a back whose size differs from its front’s or that cannot be found, and scans the fronts’ regions again at 300 ppi, 8 bits, as `back.tif` in the bag. Each photo is now a workflow of its own under its load (`scan_photo`: develop front → awaiting back → develop back → accepted → package); develop and package leave the load. The load page shows the backs preview and asks for the prints to be turned, or for the backs to be skipped when they are blank (then the bag has no `back.tif`); the photo page shows the back; the backs can be rescanned. Masters at 8 bits per channel; fakes and the recorded scanner serve the side turned to the glass (#39)
- Describe each photo (story #40 of the batch-lifecycle epic): on its page, while the load is checked, the operator says what is known of a photo — the date taken (an ISO 8601 prefix, with an approximate flag), the photographer and what the back says — each kept in photo.yaml as a claim with its source (the back or front of the print, or a relative’s word) and how it was read, as the format statement has it; nothing known is an answer. The photo workflow gains a `describe` step after `accepted`: a photo not yet described waits there, and the load page lists them; once archived the record is shown and fixed (#40)
- Deposit and receipt (story #41 of the batch-lifecycle epic, life-weaver#33): photo ids come from the archive service (`POST /people/{home}/items`), not a local series; once bagged each photo is deposited (`PUT /items/{id}`, a tar of the bag) and its receipt read back and checked against the bag, checksum for checksum; a bag the archive refuses as a duplicate ends the photo, rejected, shown on its page and the load’s; a service that is down fails the step until retried. Staging keeps bags under `bags/` with the archive’s answer beside each. The service client is `internal/archive`, the bag writer `internal/bag`; tests use `archivetest`, an in-memory service that follows life-weaver’s deposit table (#41)
- Close and purge (story #42, the last of the batch-lifecycle epic): a batch closes when scanning is finished and every photo is held by the archive or rejected by it; its work files (loads, held photos’ bags and renditions) are purged 30 days after it closes, or sooner, oldest closed first, while staging has under 10 GiB free, at startup and daily; never an open batch’s, and a rejected photo’s bag is kept. The batch page says when it closed and when it was purged (#42)

## [0.11.0] - 2026-10-24

 - A batch is a sitting of scanning over many loads of the bed (the batch-lifecycle epic, story A): New batch takes a title and starts the first load; each load has its own page, workflow (`scan_load`, a child of the batch’s) and description, pre-filled from the title; the batch page lists its loads, takes another and finishes scanning once every load is through. photo.yaml, the master’s XMP and the bag record the load with the batch. Load pages live at `batches/<id>/loads/<n>`; batches staged before this release are not read

### Changed

- A batch is a sitting of scanning over many loads of the bed (the batch-lifecycle epic, story A): New batch takes a title and starts the first load; each load has its own page, workflow (`scan_load`, a child of the batch’s) and description, pre-filled from the title; the batch page lists its loads, takes another and finishes scanning once every load is through. photo.yaml, the master’s XMP and the bag record the load with the batch. Load pages live at `batches/<id>/loads/<n>`; batches staged before this release are not read (#38)

## [0.10.0] - 2026-10-24

 - The New batch form’s Credit field has two buttons that insert `©` and `CC BY 4.0` at the cursor; A working batch’s page no longer redraws itself every 3 s: it polls a progress line naming the step running, how long it has run and, judged from the batches before (the detail pass by ppi and area scanned), about how long it should take

### Added

- The New batch form’s Credit field has two buttons that insert `©` and `CC BY 4.0` at the cursor (#36)

### Changed

- A working batch’s page no longer redraws itself every 3 s: it polls a progress line naming the step running, how long it has run and, judged from the batches before (the detail pass by ppi and area scanned), about how long it should take (#37)

## [0.9.0] - 2026-10-24

 - The register is read through the archive service's API (life-weaver#23): `bytestone-scan serve -archive URL` (default `http://localhost:8321`, `life-weaver serve`) replaces `-people DIR`; the directory reader of #25 and its fixtures go, tests use `registertest.List`. A service not answering refuses the New batch form naming it and how to start it

### Changed

- The register is read from the archive's person directories (`people/<dir>/person.yaml`, life-weaver#28); a flat `<id>.yaml` from before the tree is still read for a person without a directory (#25)
- The register is read through the archive service's API (life-weaver#23): `bytestone-scan serve -archive URL` (default `http://localhost:8321`, `life-weaver serve`) replaces `-people DIR`; the directory reader of #25 and its fixtures go, tests use `registertest.List`. A service not answering refuses the New batch form naming it and how to start it (#33)

## [0.8.0] - 2026-10-24

 - Go 1.27.3; gobank-workflow 0.11.0 (child runs, an empty registry), go-postgres 0.11.0; `go fix` runs with fmt

### Changed

- Go 1.27.3; gobank-workflow 0.11.0 (child runs, an empty registry), go-postgres 0.11.0; `go fix` runs with fmt (#28)

## [0.7.0] - 2026-10-24

 - Each workflow definition's page shows its picture, drawn from the registry by `task diagrams` and embedded; The Workflows page lists only the workflows the scanner runs, not the engine registry's end of day and release

### Added

- Each workflow definition's page shows its picture, drawn from the registry by `task diagrams` and embedded (#24)

### Fixed

- The Workflows page lists only the workflows the scanner runs, not the engine registry's end of day and release (#24)

## [0.6.0] - 2026-10-24

 - A develop step makes each master's renditions before the quality check: the print measured on the master, cropped, deskewed, levels set, as an HD display JPEG with 200 px and 64 px thumbnails under renditions/<id>/<recipe>/; bytestone-scan capture records one real print, and serve -recorded replays it as the scanner, so the UI runs on real pixels without the 4990; New batch records the prints' provenance and credit, pre-filled from the last batch and copied into every photo.yaml and the master's XMP

### Added

- A develop step makes each master's renditions before the quality check: the print measured on the master, cropped, deskewed, levels set, as an HD display JPEG with 200 px and 64 px thumbnails under renditions/<id>/<recipe>/ (#3)
- bytestone-scan capture records one real print, and serve -recorded replays it as the scanner, so the UI runs on real pixels without the 4990 (#7)
- New batch records the prints' provenance and credit, pre-filled from the last batch and copied into every photo.yaml and the master's XMP (#14)

## [0.5.0] - 2026-10-23

 - New batch chooses the home person from the register by name; a glossary on the About page (Item, Photo, Home, Album; Bag is no longer a term); the staging directory bags/ becomes archive/

### Changed

- New batch chooses the home person from the register by name; a glossary on the About page (Item, Photo, Home, Album; Bag is no longer a term); the staging directory bags/ becomes archive/ (#10)

## [0.4.0] - 2026-10-23

 - New batch says the scan ppi is averaged down to the master's ppi: a higher scan ppi takes longer but lowers noise, and the master is the same size

### Changed

- New batch says the scan ppi is averaged down to the master's ppi: a higher scan ppi takes longer but lowers noise, and the master is the same size (#8)

## [0.3.0] - 2026-10-22

 - The master's XMP says where the print and its margin lie in it: angle, centre and size in master pixels (`bs:print`, `bs:region`), so a master apart from its bag can still be straightened; masters stay unrotated (#9)

## [0.2.0] - 2026-10-21

 - Regions are square to their print: a print turned up to 10° gets a region turned with it, its close-up drawn square and its arrows moving the print's own edges; the master covers the region's bounds and is not rotated; over 10° it is flagged to straighten by hand (#4)

## [0.1.0] - 2026-10-21

 - Cancel a batch from the queue, one at a time: a Cancel button beside View on any batch not running (gobank-workflow v0.10.0) (#2)

 - Preview at 150 ppi, up from 75: the 4990 scans whole 8 px blocks across, so 75 ppi lost 2 mm of the bed's right edge; 150 loses 0.5 mm (#2)
 - Batches run from the web UI: new batch, rescan the preview, confirm, a check-quality step with accept or rescan at another ppi, back to the prints, retry; the command line's `batch`, `confirm` and `resume` are gone (#2)
 - Region margin 0.5 mm, down from 3: tight by default (#2)
 - Quality page shows the master's four corners at 1:1, with margin arrows that move an edge and rescan (#2)
 - Split a merged region, remove a region (#2)
 - Arrows on each region's edges at confirm, 0.5 mm and 3 mm, only where the edge can move; per-print close-ups (#2)
 - Navbar: About page (version, scanner, folders, links) and a scanner indicator, idle or busy with a batch; the Workflows link works (#2)
 - `bytestone-scan serve` (`task serve`): batches as the work queue, a batch's prints, and each photo's quality page: the master to fit, 1:1 crops, histogram and clipping within the print (#2)

 - Capture the fronts of a batch: preview, find prints, confirm, one detail pass at 600, 1200 or 2400 ppi averaged to the master ppi, 16-bit Deflate TIFF masters with XMP, one BagIt bag per photo (#1)

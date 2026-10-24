# Changelog

## [Unreleased]

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

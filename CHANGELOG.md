# Changelog

## [Unreleased]

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

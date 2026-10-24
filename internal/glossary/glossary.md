# Glossary

The words bytestone-scan uses, in its code, its pages and its docs.

| Term | Meaning |
|---|---|
| Item | One thing in a person's collection: a photo, letter, notebook, …; has its own id, from one series for every type |
| Photo | An item that is a photographic print; kept as a BagIt bag (RFC 8493) holding its masters and photo.yaml |
| Home | The person whose collection an item belongs to |
| Album | A titled, ordered arrangement of items, kept with a person's record on i.bytestone.uk; an item may be in many albums |
| Batch | One sitting of scanning: its title, home person, provenance, credit and scan resolution, over as many scans of the bed as the shoebox needs; finished when the operator says so and every scan is through |
| Scan | One placing of prints on the bed within a batch, previewed, confirmed, scanned front and back: its description (the batch's title unless the operator says otherwise), preview, prints, fronts pass, backs pass, masters and the photos archived from them |
| Print | A physical photograph on the bed |
| Side | Front or back of a print |
| Region | Where a print lies on the bed, with a margin, found from the preview; square to the print, turned with it up to 10° |
| Bounds | The rectangle square to the bed that covers a region: what the detail pass scans and the master covers |
| Preview | The 150 ppi bed scan regions are found from |
| Detail pass | One pass of the scanner over every region in the scan: the fronts pass at the scan ppi, then, the prints turned over in place, the backs pass at 300 ppi over the same regions |
| Scan ppi | The resolution of the fronts pass: 600, 1200 or 2400. Backs are scanned at 300 |
| Master ppi | The resolution of a master: 600, or 1200 for prints smaller than 6 × 4 inches |
| Master | The archival image of one side, never altered after capture: a front at 16 bits per channel, a back at 8 |
| Rendition | A display image derived from a master: cropped to the print, turned square, its levels set, scaled to fit a screen |
| Recipe | How renditions are made from a master, by version; a new recipe makes new renditions and leaves the old on record |
| Develop | The photo step that makes one side's renditions |
| Claim | One thing said about a photo (the date taken, the photographer, what the back says), with its source (the back or front of the print, or a relative's word) and how it was read; kept in photo.yaml, never in the master |
| Describe (a photo) | The photo step where the operator says what is known of it, as claims, or that nothing is; waits after the scan is accepted until answered |
| Deposit | The photo step that hands its bag to the archive service, which gave out its id; the answer is kept beside the bag |
| Receipt | The archive's answer that it holds a photo, the bag's checksums matching; read back from the service and checked against the bag |
| Reject | The archive's answer that it will not take a photo because it already holds the same picture, naming the item it duplicates; ends the photo, its bag left in staging |
| Work files | What a batch keeps under staging to do its job: previews, prints, masters, renditions, bags |
| Close | The batch step reached when scanning is finished and every photo is held by the archive or rejected by it: its work files are redundant from then |
| Purge | Deleting a closed batch's work files, 30 days after it closes or sooner when staging is short of space, oldest closed first; never an open batch's, and never a rejected photo's bag |

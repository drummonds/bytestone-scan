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
| Import | One received page brought into a batch, numbered among its scans: the page is its bed, previewed at 150 ppi, its prints found and confirmed as on a bed scan, each cut from the page at the page's own ppi; no backs. Its description, sender, date received and device are recorded |
| Feed | One load of the sheet feeder within a batch, numbered among its scans as an import is: its description, every side of every sheet as fed, both sides in one pass at 600 ppi and 8 bits, and the letters the operator arranges the sheets into |
| Feeder | The Epson FastFoto FF-680W as the scanner drives it: `scanimage` on the SANE `epsonds` backend, source *ADF Duplex*, colour, 8 bits, skew correction off, until the feeder is empty |
| Sheet | One piece of paper fed, two sides, kept whole; its role is `sheet` or `envelope`. The letter pages a sheet holds are cut from its sides as renditions |
| Arrange | The feed step where the operator says which sheets make which letter, in what order, each sheet's role, and which sides are blank; one load is usually one letter, so every sheet starts in letter 1, a sheet, nothing marked blank |
| Blank (a side) | A side of a sheet the operator marks at arrange as having nothing on it: left out of the letter, no master and no renditions, recorded in letter.yaml as `blank: true` with no file. A sheet blank on both sides is in no letter, and the sheets after it are numbered without a gap. The scanner never judges a side blank. Not the same as not received |
| Letter | An item of kind `letter`: its sheets' sides, those not left out as blank, as masters, `sheet-01-front.tif`, `sheet-01-back.tif`, …, with `letter.yaml`; a set of renditions per kept side under the side's own name; at least one side; deposited and receipted as a photo is |
| Letter page | One face of a letter as it is read, cut from a side: a side is one, or two for a side folded in half, left and right of its middle as turned. Each has its number in the letter's reading order, from 1 with no gap, or is marked blank and has none; kept per side in letter.yaml, left to right, as the operator says on the letter's page before the feed is accepted. A numbered page's picture is the rendition `page-NN.jpg` in its side's set. Not a Page, which is a file received |
| Folded in half | What the operator says of a side that holds two letter pages; said of one side it is said of its sheet's other side too. The cut is at the middle of the side, not where the fold is found |
| Film import | One received film brought into a batch, numbered among its scans as an import and a feed are: the file as received, the transfer, the poster chosen, the film described, then packaged and deposited as one film item. Its home, provenance and credit are its batch's. No preview, prints, regions, backs or develop |
| Film | An item of kind `film`: the file as received, never re-encoded, as `film.<ext>` under the extension it came with, with `film.yaml`; its renditions the poster's display, thumb and tiny and the play copy; deposited and receipted as a photo is |
| Transfer | Who digitised a film, when, from what medium, with a note: the film's provenance as a page's sender is, kept in film.yaml beside the batch's. The file's own date and studio are the transfer's, not the film's |
| Poster | The still that stands for a film: the picture attached in the file when it has one, the largest when it has several, or a frame at a time the operator gives; the display, thumb and tiny renditions are made from it |
| Play copy | The rendition `play.mp4` a browser plays: H.264 and AAC in MP4, its index at the front. The file's video and audio copied when they are already so, transcoded otherwise; pictures attached, data tracks and chapters left out |
| Probe | What is read off a film's file with `ffprobe`: duration, container, codecs, frame rate, size and chapters, kept under `file` in film.yaml; and, for the forms, the pictures attached and the file's tags |
| Made | When a film was made: one claim with its source, a relative's word or the film itself, and how it was read; never taken from the file's tags |
| Page | A file received from elsewhere: an image of prints captured by someone else's device, kept unaltered as sent. Holding one print, it is kept in that photo's bag; holding several, it is an item of its own with page.yaml (sender, received, device, format, ppi, the photos cut from it), deposited before them |
| Received side | A side whose master was cut from a page, not scanned here: its ppi and depth the page's, no averaging; its record carries the page's own and, when the page is an item, names it as `cut_from` |
| Not received | What a photo cut from a page says of its back: the page holds the front only. Not the same as blank |
| Print | A physical photograph on the bed |
| Side | Front or back of a print |
| Region | Where a print lies on the bed, with a margin, found from the preview; square to the print, turned with it up to 10° |
| Bounds | The rectangle square to the bed that covers a region: what the detail pass scans and the master covers |
| Preview | The 150 ppi bed scan regions are found from |
| Background | The lid or black card behind the prints on the preview, as a field: its level estimated per row and per column of the bed, so the lid's shading is background wherever it falls. A pixel over 14 levels from it on any channel is print; a speck under 1 mm is noise |
| Detail pass | One pass of the scanner over every region in the scan: the fronts pass at the scan ppi, then, the prints turned over in place, the backs pass at 300 ppi over the same regions |
| Scan ppi | The resolution of the fronts pass: 600, 1200 or 2400; for an import, the page's own. Backs are scanned at 300 |
| Master ppi | The resolution of a master: 600, or 1200 for prints smaller than 6 × 4 inches; for a received side, the page's own |
| Master | The archival image of one side, never altered after capture: a front at 16 bits per channel, a back at 8, a received side at the depth received |
| Rendition | A display image derived from a master: cropped to the print, turned square, given the side's turn, its levels set, scaled to fit a screen |
| Turn | The quarter turns a side's rendition is given so the picture is the right way up, as the operator says at confirm, from the close-up, or at the quality check, from the rendition: 90, 180 or 270 degrees clockwise, kept on the found print in prints.yaml and on the side in photo.yaml and so in the bag; a letter's side is turned on the letter's page before the feed is accepted, its sheet's other side the opposite way by as much unless this side only is said, since the feeder sees the back flipped; kept in letter.yaml; the master lies as scanned |
| Recipe | How renditions are made from a master, by version; a new recipe makes new renditions and leaves the old on record |
| Develop | The photo step that makes one side's renditions |
| Caption | What a photo shows, in the archivist's own words: "Air Raid Wardens". Not a claim: it has no source or reading; kept in photo.yaml as caption beside the scan's description, blank meaning none |
| Claim | One thing said about a photo (the date taken, the photographer, what the back says), with its source (the back or front of the print, or a relative's word) and how it was read; kept in photo.yaml, never in the master |
| Describe (a photo) | The photo step where the operator says what is known of it, as claims, or that nothing is; waits after the scan is accepted until answered |
| Deposit | The photo step that hands its bag to the archive service, which gave out its id; the answer is kept beside the bag |
| Receipt | The archive's answer that it holds a photo, the bag's checksums matching; read back from the service and checked against the bag |
| Reject | The archive's answer that it will not take a photo because it already holds the same picture, naming the item it duplicates; ends the photo, its bag left in staging |
| As sent | What the local store holds of a deposited item: its bag and the archive's answer at the time. Never rewritten; gone at purge but for the answer |
| Changed since | Of a record (`photo.yaml`, `page.yaml`, `film.yaml`, `letter.yaml`): the archive holds it sound, with another checksum than the one receipted. The archive's doing, a correction or a renumber, and no fault: Verify says so and the item is verified. A master is never changed since: another checksum there is *differs* |
| Home now | The person the archive says an item is homed with when asked, against the home as sent; Verify and the item's page show it, with *deposited under* the other when they differ |
| Re-home | Giving an open batch its home person again, when the register no longer holds the one it names: the batch names the person chosen, what is not yet deposited is written again with that home, what is deposited is left as sent. Until then the batch begins no scan, import, feed or film import, and deposits nothing |
| No id | 0: never offered, chosen or written as a person's id, nor as the item a photo duplicates; read from an older file, it means none was given or the id is gone |
| Work files | What a batch keeps under staging to do its job: previews, prints, masters, renditions, bags |
| Close | The batch step reached when scanning is finished and every photo is held by the archive or rejected by it: its work files are redundant from then |
| Purge | Deleting a closed batch's work files, 30 days after it closes or sooner when staging is short of space, oldest closed first; never an open batch's, and never a rejected photo's bag |
| Delete (a batch) | The operator's removal of a batch, open, closed or purged: its runs gone from the Queue, its work files, bags and renditions gone from staging, its id never given out again; what the archive holds stays held, what was not deposited loses its masters |
| Completed (a batch) | Closed or purged: its scanning done and every photo answered for by the archive |
| Clean up | The tool that deletes every completed batch below one row of the batches, newest first, each as Delete does, leaving the open ones; the orphans go with them |
| Orphan | What no readable batch claims, whatever the archive answered: a bag, the archive's answer or a renditions directory in staging, or a run in the Queue whose batch has no directory; left by a deletion that failed part way or a batch moved by hand; listed on Clean up, removable |
| Archive instance | One archive tree as life-weaver names it (`name`, `purpose`: production, uat, scratch or scale, `slot` 0–9); the scanner runs for one, UAT unless `-instance` names another, finds it on `localhost:8321+slot` or serving at the `-archive` URL given, takes its own port and staging from the slot, and names it in the navbar |
| Stale | What staging holds that no readable batch claims and the archive has not answered for: left by older builds or by hand; listed on the Batches page, deletable |

# bytestone-scan

Scan family photographs on an Epson Perfection 4990 into archival masters,
and from them display images for [i.bytestone.uk](https://i.bytestone.uk).
The capture follows the format statement
[Archiving family photographs](https://www.bytestone.uk/posts/2026-10-20-archiving-family-photographs/):
16-bit TIFF masters, one BagIt bag per photograph, metadata that outlives
the tool. The pipeline runs on
[gobank-workflow](https://git.bytestone.uk/hum3/gobank-workflow).

## Status

Capturing the fronts of a batch of prints works end to end on the 4990,
run from a web UI: stories [001](docs/stories/001-capture-fronts.md) and
[002](docs/stories/002-batch-ui.md).

## Use

```sh
task serve      # or: bytestone-scan serve, then open http://localhost:8080
```

A batch is one sitting of scanning, over as many scans of the bed as the
shoebox needs. Start one from *New batch*: a title, the home person
(chosen by name from the register), the prints' provenance and credit
(copied into every photo, and pre-filled from the last batch) and the
scan ppi. Its first scan starts at once; each scan's page leads through
the steps: check the prints found on the preview (or rescan it), confirm,
turn the prints over where they lie and scan the backs, check each photo's
quality and its renditions, front and back (or rescan either side, or go
back to the prints), give each photo a caption (what it shows) and say what is known of it (the
date taken, the photographer, what the back says, or nothing), accept. Each photo is then
bagged, deposited with the archive service and receipted, or shown as
rejected when the archive already holds the picture. The batch page
lists its scans, takes another, and finishes the batch once every scan is
through; it then closes, and its work files are purged 30 days later, or
sooner when staging is short of space. Any batch, open, closed or purged, is
deleted from its page or the *Batches* list, which says what the archive
holds of it (that stays) and what has not been deposited (that loses its
masters); the *Batches* page also lists the stale data no batch claims,
deletable the same way.
Renditions (story [003](docs/stories/003-display-images.md)) are kept
under `renditions/` beside the archive, by photo and recipe version.

A print that cannot reach the scanner comes as a *page*: a copy made on
an office machine and emailed, or a phone photograph (story
[102](docs/stories/102-import-received-pages.md)). *Import pages* on the
batch page (or *Import pages instead* on New batch) takes the files as
received, JPEG, PNG or TIFF, with who sent them, when and on what, and
makes each an import among the batch's scans: the page is its bed, the
prints on it are found, confirmed and cut from it at the page's own
resolution and depth, and each photo then goes through the same quality
check, description and deposit, with no backs. The file as sent is kept
unaltered: in the photo's own bag when the page holds one print, or, when
it holds several, as a page item of its own that the photos name as
`cut_from`, deposited before them. The image has to be pulled out of a
PDF or an email first.

Batches, the bags awaiting the archive, recordings and the workflow
database live under `-staging` (default `~/.local/share/bytestone-scan`).
The scanner runs for one archive instance, production unless `-instance`
names another (`bytestone-scan serve -instance h3`). It finds the instance
among the life-weaver services on `localhost:8321-8330`, one port per
slot, and refuses to start when it is not serving, saying what it found.
Its own address and staging root follow from the slot: production on
`localhost:8080` with the default staging, any other on `8080+slot` with
`~/.local/share/bytestone-scan-<instance>`, so a scratch batch's masters
never land in production's staging. `-archive URL`, `-addr` and `-staging`
each override their part. The navbar names the instance in use, in warning
colours when it is not production. The archive service (`life-weaver
serve`, or `task serve` in life-weaver) is where people come from, where
photo ids are reserved and where bags are deposited.

Without the scanner, run the UI on a recorded print: `bytestone-scan
capture -print <name>` records the one print on the bed (its preview and
its region at 2400 ppi) under `fixtures/<name>/`, once per side, and
`bytestone-scan serve -recorded <name>` replays it (story
[007](docs/stories/007-recorded-scanner.md)).

## Glossary

The words in the code, the pages and these docs are defined in
[the glossary](internal/glossary/glossary.md), also shown on the app's
About page.

## Links

| | |
|---|---|
| Documentation | https://bytestone-scan.docs.bytestone.uk |
| Source | https://git.bytestone.uk/hum3/bytestone-scan |
| Mirror | https://github.com/drummonds/bytestone-scan |

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
task serve      # for the hosted UAT archive; then open http://localhost:8081
```

At startup it says where it serves, then which scanners it found:

```
bytestone-scan: http://localhost:8080/ (staging …)
bytestone-scan: bed scanner: Epson Perfection 4990 Photo (SANE epson2), not connected
bytestone-scan: feeder: Epson FastFoto FF-680W (SANE epsonds), at epsonds:libusb:001:012
```

About shows the same. A scanner not connected is looked for again each
time a scan or a feed wants it, so it may be plugged in later; one
unplugged after it was found needs a restart.

A batch is one sitting of scanning, over as many scans of the bed as the
shoebox needs. Start one from *New batch*: a title, the home person
(chosen by name from the register), the prints' provenance and credit
(copied into every photo, and pre-filled from the last batch) and the
scan ppi. Its first scan starts at once, unless the scanner it needs is
not connected (the 4990 for a bed scan, the FF-680W for letters): then
nothing is recorded and the form comes back saying so, as it does for
one more scan or feed in a batch; each scan's page leads through
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
masters); its runs go from the Queue with it, and its id is not given
out again. The *Batches* page also lists the stale data no batch claims,
deletable the same way. *Tools → Clean up* lists the batches newest
first, each row with *Delete below*: every closed or purged batch below
that row goes, open ones stay, and with them the orphans, what no batch
claims in staging or the Queue after a deletion failed part way.
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

Letters go through the Epson FastFoto FF-680W's sheet feeder, not onto
the bed (story [117](docs/stories/117-scan-letters-through-the-ff-680w-sheet-feeder-a.md);
the format statement's *Letters* section). *Scan letters* on the batch
page (or *Scan letters instead* on New batch) feeds the sheets loaded,
both sides of each in one pass at 600 ppi and 8 bits, as a *feed* among
the batch's scans. The feeder scans the full width of its path, so each
side is cut at capture to the sheet and 3 mm either side, and the white
the driver fills below the paper's end is left off (story
[177](docs/stories/177-a-feed-on-the-real-ff-680w-fails-scanimage-s-pnm.md));
the feeder hands its pictures over as JPEG, quality 90, which the master
holds decoded. The operator then arranges the sheets into letters,
saying which sheet is which letter's and which is an envelope; one load
is usually one letter, and marking any side that is blank: a side so
marked is left out of the letter and `letter.yaml` records it as
`blank: true` with no file, a sheet blank on both sides is left out
whole, and a letter needs at least one side (story
[181](docs/stories/181-a-blank-side-is-left-out-of-a-letter-the-operator.md)).
Nothing is marked for the operator. Each letter is an item of kind
`letter`: every side kept, whole, as a master, `sheet-01-front.tif`,
`sheet-01-back.tif`, …, with `letter.yaml`, a set of renditions per kept
side, turned as the operator says on the letter's page before accepting.

A sheet is not a page, so on the letter's page, before accepting, the
operator also lays the letter out as it is read (story
[202](docs/stories/202-a-letter-s-sides-are-cut-into-pages-the-operator.md);
experimental). A side is one *letter page*, or two when marked *Folded
in half*, left and right of the middle of the side as turned; each page
is given its number in the reading order, from 1 with no gap, or marked
blank. A sheet's two sides turn and fold together, the back the opposite
way, since the feeder sees it flipped; *This side only* turns one.
`letter.yaml` lists each side's pages left to right, and each numbered
page is a rendition, `page-NN.jpg` in its side's set, cut from the
master, which is untouched; *Pages* on the letter's page shows them in
reading order. The cut is at the middle, not at the fold found. A letter
accepted before this is one page a side in sheet order. The text of each
page and a reading-order PDF are later. The feeder is found among `scanimage -L` as the `epsonds`
device, USB first; without it a feed fails at its first step and says so.

A film received, one file digitised by someone else, is imported
(story [116](docs/stories/116-import-a-film-one-received-moving-picture.md);
the format statement's *Films received* section). *Import a film* on the
batch page (or *Import a film instead* on New batch) takes one `.m4v`,
`.mp4`, `.mov`, `.avi` or `.mkv`, written to staging as it arrives, as a
*film import* among the batch's scans; nothing is previewed, found or
cut. What `ffprobe` reads off the file is shown, and its tags pre-fill
the title and the *transfer* (who digitised it, and when: the film's
provenance), since a transfer's file is dated and signed by the transfer;
when the film was made is never taken from them. The operator confirms
those and the *poster*, the picture attached in the file (the largest of
several) or a frame at a time, then says who the film names and in what
part (director, producer, camera, cast, narrator, subject), each a person
of the register, and when it was made; nothing known is an answer. The
names the file gives are shown beside the picker, not matched. The film
is an item of kind `film`: the file byte for byte as `film.<ext>` with
`film.yaml`, and as renditions the poster's `display.jpg`, `thumb.jpg`
and `tiny.jpg` and the play copy `play.mp4`, H.264 and AAC with its
index at the front, a remux when the file is that already and a
transcode otherwise. It needs `ffmpeg` and `ffprobe` on the path;
without them *Import a film* says so and refuses.

Batches, the bags awaiting the archive, recordings and the workflow
database live under `-staging` (default `~/.local/share/bytestone-scan`).
The scanner runs for one archive instance, UAT unless `-instance` names
another (`bytestone-scan serve -instance h3`): every depositor writes UAT,
and items are promoted to production from there. It finds the instance
among the life-weaver services on `localhost:8321-8330`, one port per
slot, and refuses to start when it is not serving, saying what it found.
`-archive URL` names a service elsewhere instead, and the scanner runs
for the instance serving there: `task serve` names the hosted service,
UAT on the NAS at `https://bytestone-uat.lan.drummonds.net` (life-weaver
story 188), so nothing is started to scan; `task serve:local` looks on
this machine. A service that does not answer or does not say which
instance it serves is refused, and so is an `-instance` that names
another than the one found there.
Its own address and staging root follow from the slot: production on
`localhost:8080` with the default staging, any other on `8080+slot` with
`~/.local/share/bytestone-scan-<instance>` (UAT on `8081`), so a scratch
batch's masters never land in production's staging. `-addr` and
`-staging` each override their part. The navbar names the
instance in use, in warning colours when it is not UAT. The archive service (`life-weaver
serve`, or `task serve` in life-weaver) is where people come from, where
photo ids are reserved and where bags are deposited. An id it gives out
that staging already holds a bag, a receipt or renditions under is
refused, the step saying which files: the archive has given the id out
before, so they are another item's. See that the archive holds that
item, move the files away, and Retry takes the next id.

`bytestone-scan verify BATCH[/SCAN]` asks the archive afresh whether it
still holds each photo of the scan, or of every scan of the batch, as it
was receipted: each item's receipt is compared, checksum for checksum,
with the one taken at deposit and with the bag while it is still in
staging, nothing being downloaded. A completed scan's page offers the
same as *Verify against the archive*, and asks i.bytestone.uk and
i.bytestone.org as well whether each held item's page is there: an item
reaches the first at life-weaver's `task publish` and the second at its
`task promote`.

Without the scanner, run the UI on a recorded print: `bytestone-scan
capture -print <name>` records the one print on the bed (its preview and
its region at 2400 ppi) under `fixtures/<name>/`, once per side, and
`bytestone-scan serve -recorded <name>` replays it (story
[007](docs/stories/007-recorded-scanner.md)).

## Glossary

The words in the code, the pages and these docs are defined in
[the glossary](internal/glossary/glossary.md), also shown on the app's
About page. About also links to *Statistics*: batches by state, scans,
photos by how they stand with the archive, what staging holds and the
runs in the workflow database. `GET /status` says the same as JSON for
life-weaver's status page (story 115): `version`, `instance` (the archive
instance the scanner runs for), `staging` (the root), `batches` by state,
`scans`, `photos` (`held`, `pending`, `rejected`: every item, a letter
or a film among them), `usage` (each entry at
the staging root, `name` and `bytes`), `total` bytes, and `error` when
the archive is not answering or staging cannot be read; the runs stay
on the page.

## Links

| | |
|---|---|
| Documentation | https://bytestone-scan.docs.bytestone.uk |
| Source | https://git.bytestone.uk/hum3/bytestone-scan |
| Mirror | https://github.com/drummonds/bytestone-scan |

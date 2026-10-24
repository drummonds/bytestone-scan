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

Start a batch from *New batch*: a description, the home person (chosen
by name from the register), the prints' provenance and credit (copied
into every photo, and pre-filled from the last batch) and the scan ppi.
The batch page then leads through the steps: check the prints found on
the preview (or rescan it), confirm, check each photo's quality (or
rescan at another ppi, or go back to the prints), accept.

Batches, the archive, recordings and the workflow database live under
`-staging` (default `~/.local/share/bytestone-scan`). People come from life-weaver's
register (`-people`, default `~/git/hum3/life-weaver/data/people`).

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

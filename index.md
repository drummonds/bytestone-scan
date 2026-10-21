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

Start a batch from *New batch*: a description, the home person's id and
the scan ppi. The batch page then leads through the steps: check the
prints found on the preview (or rescan it), confirm, check each photo's
quality (or rescan at another ppi, or go back to the prints), accept.

Batches, bags and the workflow database live under `-staging`
(default `~/.local/share/bytestone-scan`). People come from life-weaver's
register (`-people`, default `~/git/hum3/life-weaver/data/people`).

## Vocabulary

| Term | Meaning |
|---|---|
| Batch | One load of the scanner bed: its description, home person and scan resolution |
| Print | A physical photograph on the bed |
| Photo | The archived record of one print; has its own id |
| Side | Front or back of a print |
| Region | Where a print lies on the bed, with a margin, found from the preview |
| Preview | The 150 ppi bed scan regions are found from |
| Detail pass | The one high-resolution scan covering every region in the batch |
| Scan ppi | The resolution of the detail pass: 600, 1200 or 2400 |
| Master ppi | The resolution of a master: 600, or 1200 for prints smaller than 6 × 4 inches |
| Master | The archival image of one side, never altered after capture |
| Rendition | A display image derived from a master |
| Home | The person whose album a photo belongs to |
| Bag | A photo's archive directory (BagIt, RFC 8493) |

## Links

| | |
|---|---|
| Documentation | https://bytestone-scan.docs.bytestone.uk |
| Source | https://git.bytestone.uk/hum3/bytestone-scan |
| Mirror | https://github.com/drummonds/bytestone-scan |

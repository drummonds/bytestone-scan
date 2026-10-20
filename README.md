# bytestone-scan

Scan family photographs on an Epson Perfection 4990 into archival masters,
and from them display images for [i.bytestone.uk](https://i.bytestone.uk).
The capture follows the format statement
[Archiving family photographs](https://www.bytestone.uk/posts/2026-10-20-archiving-family-photographs/):
16-bit TIFF masters, one BagIt bag per photograph, metadata that outlives
the tool. The pipeline runs on
[gobank-workflow](https://git.bytestone.uk/hum3/gobank-workflow).

## Status

Story 1, capturing the fronts of a batch of prints, works end to end on
the 4990: [docs/stories/001-capture-fronts.md](docs/stories/001-capture-fronts.md).

## Use

```sh
bytestone-scan batch -describe "SS Bonita at her berth" -person 0 -ppi 2400
# check the annotated preview it names, then
bytestone-scan confirm 20261020-1
# after a failure, once fixed
bytestone-scan resume 20261020-1
```

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
| Preview | The 75 ppi bed scan regions are found from |
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

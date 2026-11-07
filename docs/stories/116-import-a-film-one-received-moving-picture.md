# 116 — Import a film: one received moving picture, described and deposited as a film item, no prints to find (#157)

**Status:** proposed, blocked on life-weaver story 032 (film items)
**Roadmap:** none — the format statement's *Films received* section is the spec

## Why

*King of Birds* has arrived: Niall Rankin's film, transferred by Alive
Studios in 2016 to a 1.2 GB H.264 file with a poster picture attached
and four chapters; no better master exists. The scanner is the one tool
that describes and deposits items, with the person picker, provenance
and credit already built, but its Import is for pages: a preview, prints
found, regions confirmed and cut. A film has none of that. It is one
file, kept as sent, with a record, a poster and a play copy, and goes
from the file straight to describe, package, deposit and receipt.

## Vocabulary

| Term | Meaning |
|---|---|
| Film import | A batch holding one film: its home person, provenance and credit as any batch, the file as received, the transfer, the poster chosen, the film described, then packaged and deposited as a film item. No preview, prints, regions, backs or develop |
| Transfer | Who digitised the film, when, from what medium, with a note: kept in film.yaml as its provenance, beside the batch's |
| Poster | The still that stands for the film: the picture attached in the file when it has one, or a frame at a time the operator gives; the display, thumb and tiny renditions are made from it |
| Play copy | The rendition `play.mp4` the site plays: the file remuxed when it is already H.264 and AAC in MP4, transcoded otherwise |
| Probe | What is read off the file with `ffprobe` and kept under `file` in film.yaml: duration, container, codecs, size, frame rate, chapters, and whether a picture is attached |

## What changes

- **New batch** gains *Import a film*: the file (`.m4v`, `.mp4`, `.mov`,
  `.avi`, `.mkv`), title, home person, provenance and credit as now, the
  transfer (by, date, from, note), and the poster: *attached*, offered
  when the probe finds one, or a frame time. The batch lists as a film
  import among the batches, its one item instead of scans.
- **Describe** asks for the people and their roles (director, producer,
  camera, cast, narrator, subject) through the person picker, and when
  the film was made as a claim with its source, as a photo's claims are;
  the probe's metadata (title, directors, producers, date) pre-fills what
  it can, the operator confirms it.
- **Package** writes the bag: `data/film.<ext>` as sent, `film.yaml`
  (title, people, made, transfer, poster, file); the renditions:
  `display.jpg`, `thumb.jpg`, `tiny.jpg` from the poster, `play.mp4`,
  `recipe.yaml` naming the file. `ffmpeg` makes the stills and the play
  copy, as `scanimage` drives the scanner; absent, the import refuses at
  New batch and says so.
- **Deposit** streams the bag from disk, the client's timeout sized to
  the file; receipt and close as for a photo. Staging holds the film
  until the receipt, and purge treats it as any photo's work files.
- `/status` and *Statistics* count a film among the items; the glossary
  gains Film import, Transfer, Poster, Play copy, Probe; README.
- Tests: no film in the repo; the test film is made by `ffmpeg` at test
  time from a colour source, a few seconds with two chapters and an
  attached picture, skipped when `ffmpeg` is absent.

## Acceptance

| Given | When | Then |
|---|---|---|
| a file with a picture attached and chapters | New batch, Import a film | the probe shows duration, format, chapters; the poster offers *attached* |
| a file with no picture attached | New batch, Import a film | the poster asks for a frame time; blank refused |
| the transfer's by or date blank | confirm | refused: the transfer is the film's provenance |
| the probe names directors and a date | Describe | the people and the made claim pre-filled, source *the file's own metadata*; the operator may change them |
| described | Package | the bag holds the file byte for byte as received, film.yaml, and the four renditions with recipe.yaml |
| an H.264/AAC MP4 or M4V | Package | `play.mp4` is a remux: same streams, no re-encode |
| another codec or container | Package | `play.mp4` is a transcode, H.264/AAC |
| packaged | Deposit | the archive receipts it `kind: film`; the bag's checksums match; a 1.2 GB file does not time out |
| `ffmpeg` absent | New batch | *Import a film* says it needs ffmpeg and refuses |

## Out of scope

- Adding a book or film held elsewhere as a placeholder (life-weaver
  story 033): a following story on this side, its form a cousin of this
  one with no file.
- A person named who is not in the register (the letters story).
- Cutting a film into chapters as items, subtitles, a film in several
  reels: one file, one item.
- A quality check on the play copy beyond that it was made.

## Decisions

The format statement, *Films received*; no ADR.

## Done when

- [ ] tests express the acceptance table and pass
- [ ] spec updated
- [ ] ADR written if a fork was resolved
- [ ] roadmap ticked and stamped; changelog entry
- [ ] issue closed with a link to the release

# 116 — Import a film: one received moving picture, described and deposited as a film item, no prints to find (#157)

**Status:** proposed; reviewed 2026-11-15 against life-weaver story 032 as released (the film contract, docs/archive.md) and the feed of story 117
**Roadmap:** none — the format statement's *Films received* section is the spec

## Why

*King of Birds* has arrived: Niall Rankin's film, transferred by Alive
Studios in 2016 to a 1.2 GB H.264 file with a poster picture attached
and four chapters; no better master exists. The scanner is the one tool
that describes and deposits items, with the person picker, provenance
and credit already built, but its Import is for pages: a preview, prints
found, regions confirmed and cut. A film has none of that. It is one
file, kept as sent, with a record, a poster and a play copy, and goes
from the file straight to describe, package, deposit and receipt. It
joins a batch as a page's import and a feed do: one among its scans.

## Vocabulary

| Term | Meaning |
|---|---|
| Film import | One received film brought into a batch, numbered among its scans as an import and a feed are: the file as received, the transfer, the poster chosen, the film described, then packaged and deposited as one film item. Its home, provenance and credit are its batch's. No preview, prints, regions, backs or develop |
| Film | An item of kind `film` (life-weaver story 032): the file as received as `data/film.<ext>`, with `film.yaml`; deposited and receipted as a photo is |
| Transfer | Who digitised the film, when, from what medium, with a note (`transfer`: `by`, `date`, `from`, `note`): the film's provenance as a page's sender is, kept in film.yaml beside the batch's |
| Poster | The still that stands for the film: the picture attached in the file when it has one (the largest when it has several), or a frame at a time the operator gives (`poster`: `attached`, or `frame: 00:01:23`); the display, thumb and tiny renditions are made from it |
| Play copy | The rendition `play.mp4` the site plays: H.264 and AAC in MP4, the index at the front so it starts before it has all arrived. The file's video and audio copied when they are already so, transcoded otherwise; attached pictures and data tracks left out |
| Probe | What is read off the file with `ffprobe` and kept under `file` in film.yaml: `duration` (hh:mm:ss), `container`, `video`, `audio`, `frame_rate`, `size`, `chapters` (`title`, `start`, `end`); and, for the form only, the pictures attached and the file's tags |

## The record

film.yaml, as life-weaver reads it (its site: `title`, `transfer`,
`people`, `made`, `file`, `credit`; its service: `title`, `people`,
`made.value`), with the scanner's own fields as letter.yaml has them:

```yaml
id: 6
batch: <batch id>
film: 1
title: King of Birds
home: {id: 1nq, name: Niall Rankin}
provenance: <the batch's>
credit: <the batch's>
transfer: {by: Alive Studios, date: 2016-03-07, from: <medium>, note: ""}
poster: {attached: true}          # or {frame: "00:01:23"}
people:
  - {person: 1nq, role: director}
made: {value: <year>, approximate: true, source: <who says>, read: ""}
file:
  name: King of Birds.m4v
  duration: "00:15:36"
  container: mp4
  video: h264
  audio: aac
  frame_rate: 25
  size: 1184285936
  chapters:
    - {title: Start, start: "00:00:00", end: "00:06:28"}
```

## What changes

- **The batch page** gains *Import a film* beside *Import* and *Feed*:
  the file (`.m4v`, `.mp4`, `.mov`, `.avi`, `.mkv`), the title, the
  transfer (by, date, from, note), and the poster: *attached*, offered
  when the probe finds one, or a frame time. The upload is written to
  staging as it arrives, never held whole in memory (a page's import
  reads its file whole; a film's must not). The film lists among the
  batch's scans, its one item where a scan has prints.
- **The probe pre-fills** what the file says and no more: the title from
  its `title` tag; the transfer's date from its creation time and its
  `by` from the studio or copyright tag, since a transfer's file is
  dated and signed by the transfer, not the film. The directors and
  producers it names are shown as written beside the person picker,
  their role chosen; the operator picks the person. Names are not
  matched to the register. `made` is never pre-filled.
- **Describe** asks for the people and their roles (director, producer,
  camera, cast, narrator, subject) through the person picker, and when
  the film was made: one `made` claim shaped as a photo's claims are
  (`value`, `approximate`, `source`, `read`), optional. It shows the
  poster as chosen, and the poster may be changed there until the film
  is packaged.
- **Package** writes the bag: `data/film.<ext>` as sent and
  `data/film.yaml`; and the renditions at the recipe root, as the
  archive wants a film's: `display.jpg`, `thumb.jpg`, `tiny.jpg` from
  the poster, `recipe.yaml` naming the file as master, and `play.mp4`.
  `ffmpeg` makes the stills and the play copy, as `scanimage` drives the
  scanner; absent, *Import a film* refuses and says so.
- **Deposit** is the photo's: the tar is already piped from disk and the
  client sets no timeout, so nothing is added but the proof that neither
  the import nor the deposit holds the file in memory. Receipt, close
  and verify as for a photo. Staging holds the film until the receipt,
  and purge treats it as any scan's work files.
- `/status` and *Statistics* count a film among the items; the glossary
  gains Film import, Film, Transfer, Poster, Play copy, Probe; README.
- Tests: no film in the repo; the test films are made by `ffmpeg` at
  test time from a colour source, a few seconds each: H.264/AAC with two
  chapters and an attached picture, and one in another codec with none.
  They run the real `ffmpeg` and `ffprobe`, since a fake cannot show a
  remux from a transcode, and are skipped when `ffmpeg` is absent; the
  archive is `archivetest`, taught the film's rendition rule.

## Acceptance

| Given | When | Then |
|---|---|---|
| a file with a picture attached and two chapters | Import a film | the probe shows duration, format and both chapters; the poster offers *attached*; the film is a scan of the batch, numbered among them |
| a file with two pictures attached | Import a film, poster *attached* | the poster is the larger |
| a file with no picture attached | Import a film | the poster asks for a frame time; blank refused |
| a frame time past the film's end, or not hh:mm:ss | confirm | refused, the duration said |
| a file `ffprobe` finds no video in (a JPEG, a text file) | Import a film | refused: not a film; nothing kept in staging |
| the title, or the transfer's by or date, blank | confirm | refused: a film has a title, and the transfer is its provenance |
| a finished batch | Import a film | refused, as a scan or an import is |
| `ffmpeg` or `ffprobe` absent | Import a film | says it needs ffmpeg and refuses |
| the file tagged with a title, a creation time, a studio and a director | Import a film | title, transfer date and transfer by pre-filled; the director's name shown as written with role *director*, no person chosen; `made` blank |
| a role with no person picked | Describe, confirm | refused: a part needs its person |
| no people and no `made` | Describe, confirm | accepted |
| a frame time given at Describe in place of *attached* | Package | the stills are of that frame; film.yaml says `poster: {frame: …}` |
| described | Package | the bag holds the file byte for byte as received (its sha256 in the manifest that of the file sent), film.yaml as *The record*, and at the recipe root `display.jpg`, `thumb.jpg`, `tiny.jpg`, `recipe.yaml` naming `film.<ext>`, `play.mp4` |
| an H.264/AAC MP4 or M4V with an attached picture | Package | `play.mp4` holds the same video and audio packets, not re-encoded, and no other stream; its index is at the front |
| another codec or container | Package | `play.mp4` is H.264/AAC, one video and one audio stream |
| packaged | Deposit | the archive receipts it `kind: film`; the bag's checksums match; verify passes |
| a film of 256 MiB made as it is sent | Import a film, then Deposit | the scanner's allocations stay under a quarter of the file (skipped under `-short`) |
| the same file imported again and deposited | Deposit | the archive's `duplicate` refusal shown, as for a photo |
| a batch holding a film, closed and past its grace | purge | the film's work files go as a scan's do |
| a batch with one film receipted | `/status`, *Statistics* | one item counted, a film |

## Out of scope

- Adding a book or film held elsewhere as a placeholder (life-weaver
  story 033): a following story on this side, its form a cousin of this
  one with no file.
- A person named who is not in the register (the letters story); a name
  the file gives is shown, not matched.
- Cutting a film into chapters as items, subtitles, a film in several
  reels: one file, one item.
- A quality check on the play copy beyond that it was made.
- One copy of the file in staging rather than the upload and the bag's:
  staging holds it twice, and the play copy, until purge.

## Decisions

The format statement, *Films received*; no ADR.

A film import is a scan of a batch, not a batch of its own, as story 117
made a feed: the batch carries home, provenance and credit once. The
file comes by upload, streamed, rather than as a path on the scanner's
machine, so the form is the import's and the server need not share a
disk with the browser. The film tests run the real `ffmpeg`, where the
scanner and feeder are faked: it is software, the same on every run.
The file's date tag is the transfer's and pre-fills that, never `made`
(*King of Birds* is tagged 2016).

## Done when

- [ ] tests express the acceptance table and pass
- [ ] spec updated (glossary, README)
- [ ] changelog fragment
- [ ] issue closed with a link to the release

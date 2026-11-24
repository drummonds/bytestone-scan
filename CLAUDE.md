# bytestone-scan — project notes

Read the global instructions first; this file adds only what is repo-specific.

- The format statement (bytestoneblog, "Archiving family photographs") is the
  spec; tests pin its rules. Change the statement before changing a rule.
- Masters are never altered after capture. Anything that improves a picture
  belongs to a rendition.
- The glossary (`internal/glossary/glossary.md`, shown on About) is the
  ubiquitous language: Item, Photo, Home, Album, Batch, Print, Side, Region,
  Preview, Detail pass, Master, Rendition; for a film: Film import, Transfer,
  Poster, Play copy, Probe; for a writing: Writing import, Convert, Left
  out, Transmission. Bag is the format a Photo is kept
  in, not a term. Archive model: life-weaver#19.
- The scanner is driven through `scanimage` (SANE `epson2`); letters go
  through the FF-680W's sheet feeder (`internal/fastfoto`, SANE
  `epsonds`), a feed among a batch's scans. Tests never touch either:
  they use fakes (`batchtest.Scanner`, `batchtest.Feeder`).
- A film received is imported through `ffprobe` and `ffmpeg`
  (`internal/film`), a film import among a batch's scans. Its tests run
  the real tools on films made at test time (`filmtest`), skipped where
  ffmpeg is absent: a fake cannot tell a remux from a transcode.
- A writing received as a Word document or plain text is converted to
  djot by allowlist (`internal/writing`), a writing import among a
  batch's scans; the document is never opened by an office program and
  never kept. No document goes in the repo: tests zip theirs from XML
  (`writingtest`). The page shown is life-weaver's own rendering
  (its `writing` package, a module dependency).
- Images never go in the repo. Bags are built under the staging root
  (`staging/` locally, gitignored) and deposited with the archive service;
  staging holds a photo only until the archive receipts it.
- Person ids are life-weaver's, lower-case Crockford base32. The register
  is read through the archive service's API (`life-weaver serve`, `-archive`
  URL), never from the archive's tree; tests use `archivetest`. The
  scanner's `internal/archive` is that service's client; `internal/bag`
  writes the BagIt bags.

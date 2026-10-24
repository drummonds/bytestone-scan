# bytestone-scan — project notes

Read the global instructions first; this file adds only what is repo-specific.

- The format statement (bytestoneblog, "Archiving family photographs") is the
  spec; tests pin its rules. Change the statement before changing a rule.
- Masters are never altered after capture. Anything that improves a picture
  belongs to a rendition.
- The glossary (`internal/glossary/glossary.md`, shown on About) is the
  ubiquitous language: Item, Photo, Home, Album, Batch, Print, Side, Region,
  Preview, Detail pass, Master, Rendition. Bag is the format a Photo is kept
  in, not a term. Archive model: life-weaver#19.
- The scanner is driven through `scanimage` (SANE `epson2`). Tests never
  touch the scanner: they use a fake that serves image files.
- Images never go in the repo. The archive is written under the staging root
  (`staging/` locally, gitignored) until the NAS is mounted.
- Person ids are life-weaver's, lower-case Crockford base32. The register
  is read through the archive service's API (`life-weaver serve`, `-archive`
  URL), never from the archive's tree; tests use `registertest.List`.

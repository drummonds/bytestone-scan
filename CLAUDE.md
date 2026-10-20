# bytestone-scan — project notes

Read the global instructions first; this file adds only what is repo-specific.

- The format statement (bytestoneblog, "Archiving family photographs") is the
  spec; tests pin its rules. Change the statement before changing a rule.
- Masters are never altered after capture. Anything that improves a picture
  belongs to a rendition.
- README's vocabulary is the ubiquitous language: Batch, Print, Photo, Side,
  Region, Preview, Detail pass, Master, Rendition, Home, Bag.
- The scanner is driven through `scanimage` (SANE `epson2`). Tests never
  touch the scanner: they use a fake that serves image files.
- Images never go in the repo. Bags are written under the staging root
  (`staging/` locally, gitignored) until the NAS is mounted.
- Person ids are life-weaver's (`../life-weaver/data/people/<id>.yaml`),
  lower-case Crockford base32.

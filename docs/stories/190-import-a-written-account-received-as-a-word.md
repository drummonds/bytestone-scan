# 190 — Import a written account received as a Word document or plain text: kept as an item as received, shown as an HTML page with a footnote, the people linked to it and its transmission recorded, and found by a search that starts from a person (#190)

**Roadmap:** none — the format statement's *Writings received* section is the spec; needs life-weaver story 336 (an account is a writing) released

Rewritten 2026-11-20: the Word document is converted at import and not
kept (the title is the issue's and still says *as received*).
Rewritten 2026-11-22: the master is djot, `writing.dj`, not Markdown;
0 is no id (story 191), so the examples name real people.
Rewritten 2026-11-24: the item is a *writing*, not an account
(life-weaver story 336): the kind, `writing.dj`, `writing.yaml` and
the package follow that name. The title is the issue's and still says
*account*.

## Why

Relatives send writing as well as pictures: a memoir typed up in Word,
a family history, a eulogy, a transcript of a diary. The scanner is the
one tool that describes and deposits items, with the person picker,
provenance and credit already built, but everything it takes in is a
picture or a film. A text that arrives as text has nowhere to go.

A Word document cannot be kept as a film's file is. It is a container
that can attach a remote template, embed objects and carry fields that
act when it is opened, and the archive should hold nothing unsafe to
open. So this is the one import that converts: the scanner reads the
words out into djot, carrying across only what it knows to be text,
and the djot is the master. The document is deleted once the bag is
written; its name, size and sha256 stay in the record. Nothing in the
scanner opens the document with Word or any office program: it is read
as a zip of XML, and only for its text.

## Vocabulary

| Term | Meaning |
|---|---|
| Writing import | One received piece of writing brought into a batch, numbered among its scans as an import, a feed and a film import are: the file received, converted, read by the operator, described, then packaged and deposited as one writing. Its home, provenance and credit are its batch's. No preview, prints, regions, backs, develop or renditions |
| Writing | An item of kind `writing` (life-weaver stories 288, 336): `data/writing.dj`, the master, with `writing.yaml`, and no other file; deposited and receipted as a photo is |
| Convert | The import step that writes the master from the received file: an allowlist, so that what is not named here has no way across. From `.docx`: headings, paragraphs, bold and italic, list items, line breaks, links to `http`, `https` and `mailto`, footnotes and endnotes. From `.txt`: its paragraphs. Every character of the text is escaped so that it means itself in djot |
| Left out | What Convert met and did not carry: counted as pictures, tables (their cells' text is kept, as paragraphs) and objects (anything embedded or linked from outside: OLE objects, attached templates, external targets other than a web link). Shown to the operator and kept in the record |
| Received | The file as it came, for the record only: its name, format, size and sha256. Held in staging until the bag is written, then deleted |
| Transmission | The hands the text passed through to reach the archive, in order: each a part (transcribed, typed, edited, kept, sent), by whom, when, with a note. By whom is a person picked from the register, or a name as written. Kept in writing.yaml beside the batch's provenance |
| Written | When the writing was written: one claim with its source and how it was read, as a film's made is; never taken from the file's own dates |

## The record

writing.yaml, as life-weaver story 288 reads it, with the scanner's own
fields as film.yaml has them:

```yaml
id: 7
batch: <batch id>
writing: 1
title: Recollections of Megginch
home: {id: 1, name: …}
provenance: <the batch's>
credit: <the batch's>
people:
  - {person: 1, role: author}
  - {person: 2, role: subject}
written: {value: 1974, approximate: true, source: <who says>, read: ""}
transmission:
  - {part: typed, by: 3, date: 1998, note: from the manuscript}
  - {part: sent, by: Jane Smith, date: 2026-11-02, note: by email}
received:
  name: Recollections.docx
  format: docx
  size: 48213
  sha256: <of the file as received>
  left_out: {pictures: 2, tables: 1, objects: 0}
```

## What changes

- **The batch page** gains *Import a writing* beside *Import*, *Feed*
  and *Import a film*: the file (`.docx`, `.txt`) and the title, which
  starts as the file's name without its extension. The writing lists
  among the batch's scans, its one item where a scan has prints.
- **Convert** (`internal/writing`) writes the djot. A `.docx` is
  opened as a zip and only `word/document.xml`, its styles, its
  relationships and its footnotes and endnotes are read, each up to a
  fixed size, so a zip made to swell cannot fill the machine. Tracked
  changes are read as accepted. A `.txt` is UTF-8 when its bytes are
  valid UTF-8 and Windows-1252 otherwise; paragraphs part at blank
  lines. A `.docm`, or a `.docx` holding a macro project, is refused
  outright: it was not sent as a plain document and the operator should
  know.
- **Read** shows the page as the site will make it from the djot
  (life-weaver's `writing` package, the module a dependency from here
  on), the count of words and notes, and what was left out, so the
  operator sees that the right file came, came whole, and what it has
  lost. The operator goes on or cancels; the djot is not edited
  here.
- **Describe** asks, through the person picker, for the people and
  their roles (author, subject); when it was written, one claim,
  optional; and the transmission, a row a hand: the part, a person
  picked or a name typed, a date, a note; rows added, removed and
  reordered. Nothing is required.
- **Package** writes the bag: `data/writing.dj` and
  `data/writing.yaml`, no renditions and no recipe; then deletes the
  received file from staging.
- **Deposit**, receipt, reject, close and verify as for a photo; verify
  looks for the writing's page on the site. `writing.yaml` is a record
  as `photo.yaml` is (story 191): changed since at Verify is no fault,
  and Re-home writes it again before its deposit. Purge treats the writing
  as any scan's work files.
- `/status` and *Statistics* count a writing among the items; the
  glossary gains Writing import, Writing, Convert, Left out, Received,
  Transmission, Written; README; `archivetest` is taught the writing's
  rule. No Word document is kept in the repo: the tests zip theirs from
  XML at test time.

## Acceptance

Convert, a `.docx`:

| The document holds | The djot, rendered, has | Left out |
|---|---|---|
| a *Heading 1*, two paragraphs, a word in italic and one in bold | a heading, two paragraphs, the words emphasised and strong | nothing |
| a heading in a style whose id is another language's and whose name is `heading 2` | a heading at level 2 | nothing |
| a footnote in one paragraph, an endnote in a later one, a footnote never referred to | notes 1 and 2 in the order referred to; the third not there | nothing |
| a note referred to twice | one note, two references | nothing |
| three list paragraphs | one list of three items | nothing |
| text holding `<script>`, `&`, `*`, `_`, `` ` ``, `^`, `~`, `{=html}`, `{.x}`, `:a:`, `[x](y)`, `[^1]`, a line starting `# `, one starting `1. ` and one starting `:::` | those characters as text: no element, emphasis, verbatim, raw, attribute, symbol, link, note, heading, list or division made of them | nothing |
| a link to `https://…` and one to `mailto:…` | both links | nothing |
| a link to `javascript:…`, one to `file://…`, one to a bookmark in the document | each its text alone | nothing |
| an insertion and a deletion tracked, a passage moved | the inserted words and the passage where it was moved to; not the deleted | nothing |
| a picture, and a text box | neither, nor the text box's words | pictures: 1 (the text box is not counted) |
| a table of two rows of two cells | four paragraphs, row by row | tables: 1 |
| an embedded spreadsheet (OLE), an attached template at a URL | neither | objects: 2 |
| a comment, a header and a footer | none of them | nothing |
| a line break and a tab within a paragraph | the break as a break, the tab as a space | nothing |

Convert, other files:

| The file | Then |
|---|---|
| `.txt`, UTF-8, three blocks parted by blank lines, a line break inside one, text holding `<b>`, `*` and `{=html}` | three paragraphs, the break kept, those characters as text |
| `.txt`, UTF-8 with a byte-order mark | the mark not in the djot |
| `.txt`, not valid UTF-8, holding byte `0xE9` | the djot is UTF-8 and has `é` |
| a `.docx` that is not a zip, or a zip without `word/document.xml` | refused, saying which |
| a `.docx` holding `word/vbaProject.bin`, or a `.docm` | refused: it carries macros |
| a `.docx` whose `word/document.xml` unzips past the limit | refused: too large to read; no more than the limit was read |
| a file with no text | refused: nothing to read |
| a file that is neither `.docx` nor `.txt` | refused: a Word document or plain text |
| any refusal | nothing of the file is kept in staging |

The import:

| Given | When | Then |
|---|---|---|
| a `.docx` with a heading, two paragraphs, a footnote and a picture | Import a writing | the title is the file's name; Read shows the page with note 1 at the foot, the count of words and notes, and *1 picture left out*; the writing is a scan of the batch, numbered among them |
| the title blank | confirm | refused: a writing has a title |
| a finished batch | Import a writing | refused, as a scan or an import is |
| a role with no person picked | Describe, confirm | refused: a part needs its person |
| a transmission row with a part and neither a person nor a name | Describe, confirm | refused: a hand needs a name |
| a transmission row with a person picked and a name typed | Describe, confirm | refused: one or the other |
| a transmission row with a name, no date and no note | Describe, confirm | accepted |
| no people, no `written`, no transmission | Describe, confirm | accepted |
| author 1, subject 2, typed by 3 in 1998, sent by *Jane Smith* on 2026-11-02, the rows in that order | Package | writing.yaml as *The record*: the transmission in the order given, `by: 3` an id and `by: Jane Smith` a name; `received` the file's name, format, size, its sha256 as received, and what was left out |
| described | Package | the bag's payload is `writing.dj` and `writing.yaml` and nothing else; no renditions; the received file is gone from staging and no copy of it is anywhere under the batch |
| the same file imported twice | Package, both | the two `writing.dj` are byte for byte the same |
| packaged | Deposit | the archive receipts it `kind: writing`; the bag's checksums match; verify passes |
| receipted, `writing.yaml` another checksum at the archive since | Verify | verified: the record has changed since (story 191) |
| packaged and not deposited, the batch's person gone | Re-home | `writing.yaml` written again with the home chosen; `writing.dj` byte for byte the same |
| the archive already holds the same djot | Deposit | rejected, naming the item it duplicates; the writing ends as a rejected photo does |
| a batch holding a writing, closed and past its grace | purge | the writing's work files go as a scan's do |
| a batch with one writing receipted | `/status`, *Statistics* | one more item counted held, among the photos as a film is |

## Out of scope

- The writing's page, its notes and provenance note, and where it shows
  on a person's page: life-weaver story 288. Its renderer passes no raw
  HTML, picture or unsafe link whatever the djot holds, so the page
  does not rest on this converter alone.
- The search by person: life-weaver#289.
- Keeping the Word document, purified or not, and OpenDocument: a
  cleaned container is still a container, and `.odt` carries macros
  too.
- The document's pictures brought in as photos, and its tables as
  tables: counted and said, not carried.
- Editing the djot before deposit, and importing a `.dj` written by
  hand.
- Pre-filling from the document's own properties (its author, its
  dates): they are the typing's, and `docProps` is not read.
- People, dates and events proposed from the text by a model: the
  reading-letters work.
- Other formats (`.doc`, `.odt`, `.rtf`, `.pdf`): each a converter of
  its own when one arrives.
- A name typed in the transmission becoming a person in the register
  (#172).

## Decisions

The format statement, *Writings received*; no ADR expected.

The master is djot (decided 2026-11-22: text the archive stores is
djot, a letter's reading to come as well; life-weaver story 331), and
no templating: a writing is shown as written. The Word document is not kept, the one
import where *kept as received* gives way: what is received is a
program's container, not the text. Convert is an allowlist, not a
cleaner: it names what it carries, so a thing it has never heard of is
left out, where a blocklist would let it through. Macros are refused,
not stripped, because a document with macros was not what the sender
said it was. The same file converts to the same bytes, so the archive's
duplicate rule works on the djot; a later converter may write other
bytes for it, and then the rule will not see the two as one.

A writing import is a scan of a batch, as a film import is: the batch
carries home, provenance and credit once.

The scanner takes life-weaver's module for the rendering, where until
now it has spoken to life-weaver only through the archive service, so
that Read shows the very page the site will make.

The transmission is a list the operator orders, not a fixed set of
fields: one text was written, kept and sent; another dictated,
transcribed, typed, edited and sent on twice.

## Done when

- [x] tests express the acceptance tables and pass
- [ ] spec updated (the format statement's *Writings received*; glossary; README)
- [x] ADR written if a fork was resolved (none was)
- [x] changelog fragment
- [ ] issue closed with a link to the release

# 190 — Import a written account received as a Word document or plain text: kept as an item as received, shown as an HTML page with a footnote, the people linked to it and its transmission recorded, and found by a search that starts from a person (#190)

**Roadmap:** none — the format statement's *Accounts received* section, to be written first, is the spec; needs life-weaver story 288 (account items) released

## Why

Relatives send writing as well as pictures: a memoir typed up in Word,
a family history, a eulogy, a transcript of a diary. The scanner is the
one tool that describes and deposits items, with the person picker,
provenance and credit already built, but everything it takes in is a
picture or a film. A text that arrives as text has nowhere to go.

An account joins a batch as a film does: one file, kept as sent, with a
record, straight to describe, package, deposit and receipt. It has
nothing to preview, find, cut or develop, and unlike a film nothing to
render: life-weaver makes the page, its footnotes and its search from
the file it holds (its stories 288 and 289). What the scanner adds is
who: the author and subjects, and each hand the text passed through.

## Vocabulary

| Term | Meaning |
|---|---|
| Account import | One received piece of writing brought into a batch, numbered among its scans as an import, a feed and a film import are: the file as received, read, described, then packaged and deposited as one account item. Its home, provenance and credit are its batch's. No preview, prints, regions, backs, develop or renditions |
| Account | An item of kind `account` (life-weaver story 288): the file as received as `data/account.docx` or `data/account.txt`, with `account.yaml`; deposited and receipted as a photo is |
| Reading | The text as life-weaver reads it off the file, by its own package: headings, paragraphs, list items and the author's notes. Shown to the operator before describing; never kept |
| Transmission | The hands the text passed through to reach the archive, in order: each a part (transcribed, typed, edited, kept, sent), by whom, when, with a note. By whom is a person picked from the register, or a name as written. The account's provenance as a film's transfer is, kept in account.yaml beside the batch's |
| Written | When the account was written: one claim with its source and how it was read, as a film's made is; never taken from the file's own dates |

## The record

account.yaml, as life-weaver story 288 reads it, with the scanner's own
fields as film.yaml has them:

```yaml
id: 7
batch: <batch id>
account: 1
title: Recollections of Megginch
home: {id: 0, name: …}
provenance: <the batch's>
credit: <the batch's>
people:
  - {person: 0, role: author}
  - {person: 1, role: subject}
written: {value: 1974, approximate: true, source: <who says>, read: ""}
transmission:
  - {part: typed, by: 2, date: 1998, note: from the manuscript}
  - {part: sent, by: Jane Smith, date: 2026-11-02, note: by email}
file: {name: Recollections.docx, format: docx, size: 48213}
```

## What changes

- **The batch page** gains *Import an account* beside *Import*, *Feed*
  and *Import a film*: the file (`.docx`, `.txt`) and the title, which
  starts as the file's name without its extension. The account lists
  among the batch's scans, its one item where a scan has prints.
- **Read** shows the reading, as the site will show it, with its notes
  at the foot, and a count of words and notes, so the operator sees
  that the right file came and came whole. The reading is life-weaver's
  package, the module a dependency from here on, so that the scanner
  never accepts what the archive will refuse. A file the reading
  refuses is refused with its reason and nothing is kept.
- **Describe** asks, through the person picker, for the people and
  their roles (author, subject); when it was written, one claim,
  optional; and the transmission, a row a hand: the part, a person
  picked or a name typed, a date, a note; rows added, removed and
  reordered. Nothing is required: an account of unknown hand is still
  an account.
- **Package** writes the bag: `data/account.<ext>` as sent and
  `data/account.yaml`. No renditions and no recipe.
- **Deposit**, receipt, reject, close and verify as for a photo; verify
  looks for the account's page on the site. Staging holds the file
  until the receipt, and purge treats it as any scan's work files.
- `/status` and *Statistics* count an account among the items; the
  glossary gains Account import, Account, Reading, Transmission,
  Written; README; `archivetest` is taught the account's rule.

## Acceptance

| Given | When | Then |
|---|---|---|
| a `.docx` with a heading, two paragraphs and a footnote | Import an account | the title is the file's name; Read shows the heading, the paragraphs, note 1 at the foot, and the count of words and notes; the account is a scan of the batch, numbered among them |
| a `.txt` of three blocks parted by blank lines | Import an account | Read shows three paragraphs, no notes |
| a `.docx` that is not a Word document, or a file with no text | Import an account | refused with the reading's reason; nothing kept in staging |
| a file that is neither `.docx` nor `.txt` | Import an account | refused: a Word document or plain text |
| the title blank | confirm | refused: an account has a title |
| a finished batch | Import an account | refused, as a scan or an import is |
| a role with no person picked | Describe, confirm | refused: a part needs its person |
| a transmission row with a part and neither a person nor a name | Describe, confirm | refused: a hand needs a name |
| a transmission row with a person picked and a name typed | Describe, confirm | refused: one or the other |
| a transmission row with a name, no date and no note | Describe, confirm | accepted |
| no people, no `written`, no transmission | Describe, confirm | accepted |
| author 0, subject 1, typed by 2 in 1998, sent by *Jane Smith* on 2026-11-02, the rows in that order | Package | account.yaml as *The record*, the transmission in the order given, `by: 2` an id and `by: Jane Smith` a name |
| described | Package | the bag holds the file byte for byte as received (its sha256 in the manifest that of the file sent) as `account.docx` or `account.txt`, and account.yaml; no renditions; the upload is gone from the import |
| packaged | Deposit | the archive receipts it `kind: account`; the bag's checksums match; verify passes |
| the archive already holds the same file | Deposit | rejected, naming the item it duplicates; the account ends as a rejected photo does |
| a batch holding an account, closed and past its grace | purge | the account's work files go as a scan's do |
| a batch with one account receipted | `/status`, *Statistics* | one more item counted held, among the photos as a film is |

## Out of scope

- The account's page, its footnotes and provenance note, and where it
  shows on a person's page: life-weaver story 288.
- The search by person: life-weaver#289.
- Pre-filling from the document's own properties (its author, its
  dates): a Word file's author is whoever's copy of Word saved it, and
  its dates are the typing's.
- People, dates and events proposed from the text by a model: the
  reading-letters work, which this gives a second kind of item to read.
- Other formats (`.doc`, `.odt`, `.rtf`, `.pdf`): as life-weaver reads
  them.
- A name typed in the transmission becoming a person in the register
  (#172).
- Changing an account's people or transmission once it is held:
  bytestone-editor's, when the archive service takes such a change.

## Decisions

The format statement, *Accounts received*; no ADR expected.

An account import is a scan of a batch, as a film import is: the batch
carries home, provenance and credit once.

The scanner takes life-weaver's module for the reading, where until now
it has spoken to life-weaver only through the archive service. The
alternative, a route on the service that returns the reading of an
uploaded file, keeps the one client boundary but sends the file twice
and makes Read fail when the service is slow. The package is small and
has no dependency on the archive.

The transmission is a list the operator orders, not a fixed set of
fields: one text was written, kept and sent; another dictated,
transcribed, typed, edited and sent on twice.

## Done when

- [ ] tests express the acceptance table and pass
- [ ] spec updated (the format statement's *Accounts received*; glossary; README)
- [ ] ADR written if a fork was resolved
- [ ] changelog fragment
- [ ] issue closed with a link to the release

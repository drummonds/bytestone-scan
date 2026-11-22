# 191 — The scanner's local store still names person 0 as home in 41 bags (~/.local/share/bytestone-scan*/bags): Victoria Drummond is person 1 since life-weaver story 313, and 0 is to be no id (life-weaver#309); what a local bag is for after its deposit, and w… (#191)

**Roadmap:** none — a fix: life-weaver story 313 left the scanner out of scope on the ground that it holds no id of hers, and it does

## Why

life-weaver moved Victoria Drummond from 0 to 1 (its story 313) and
item 0 to another id (322), rewriting each record that named them under
its bag's manifests, so that 0 can come to mean nobody (#309). The
scanner's local store was not walked, and names 0 in three places:

| Where | Says | How many, 2026-11-21 |
|---|---|---|
| `batches/<id>/batch.yaml` | `person: "0"` | 12 batches, 10 closed and 2 open |
| `bags/<item>/data/photo.yaml` | `home: {id: "0"}` | 41 bags, all deposited |
| `bags/<item>.deposit.yaml` | `home: "0"`, and `duplicate_of: "0"` on an item that is no duplicate | every deposit of hers; the second on every held deposit |

Three things follow. Verify now calls every one of her items
*differs*: the archive's `photo.yaml` is the rewritten one, its
checksum not the one receipted (item `1r` in UAT: receipted `4c81…`,
held `a7ce…`, the master the same). The same false alarm is raised by
any correction bytestone-editor makes, so it was there before the
renumber and the renumber only made it plain. The Verify page links
her items under `/0/`, which is gone. And an open batch that names 0
would reserve ids for nobody and write 0 into new records.

The question the idea asks is what a local bag is for once deposited.
It is a work file: the copy of what was sent, kept so the deposit can
be checked and repeated until the batch is purged. It is not the
archive's record and does not follow it. So nothing local is rewritten;
what the local store says is what was said at the time, and what is
true now is asked of the archive.

## Vocabulary

| Term | Meaning |
|---|---|
| As sent | What the local store holds of a deposited item: its bag and the archive's answer at the time. Never rewritten; gone at purge but for the answer |
| Changed since | Of a record (`photo.yaml`, `page.yaml`, `film.yaml`, `letter.yaml`): the archive holds it sound, with another checksum than the one receipted. The archive's doing, a correction or a renumber, and no fault. A master is never changed since: for a master, another checksum is *differs* |
| Home now | The person the archive says an item is homed with when asked, against the home as sent |
| Re-home | Giving an open batch its home person again, when the register no longer holds the one it names |
| No id | 0: never offered, chosen or written as a person's or an item's id; read from an older file, it means none was given or the id is gone |

## What changes

- **Verify** tells a record from a master. A record whose checksum at
  the archive is not the one receipted, the archive's own fixity sound,
  is *verified*, and the check says the record has changed since the
  deposit; a master that differs is *differs* as now. The comparison
  with the bag in staging is of masters alone.
- **Home now**: Verify and the photo's page show and link the home the
  archive gives, and say *deposited under 0* beside it when the answer
  at the time named another. The site link is built from the home now.
- **An open batch whose person the register does not hold** refuses a
  new scan, import, feed or film import, and a deposit, saying who is
  gone, and offers *Re-home*: the person picker. The batch then names
  the person chosen; photos packaged and not yet deposited are packaged
  again with that home; what is deposited is left as sent.
- **A deposit is not sent for a home the archive does not give the
  id**: before it sends, the scanner asks who the reserved id is homed
  with, and where that is not the bag's home it refuses and offers
  *Re-home*. This is what keeps a stale 0 out of the archive.
- **No id**: the person picker and every form refuse 0. `batch.yaml`
  and a deposit answer naming 0 still load; the batch list and the
  pages show such a home as *0, no longer an id*. A deposit answer is
  written without `duplicate_of` when the item is no duplicate, and one
  read with `duplicate_of: "0"` and `status: held` is no duplicate.
- Glossary: As sent, Changed since, Home now, Re-home, No id; README.
- Nothing is migrated. The 41 bags go at purge, 30 days from their
  batch's close; their batches and answers stay, saying what was said.

## Acceptance

Verify, an item deposited and receipted, its bag still in staging:

| At the archive | The archive's fixity | Verdict | Says |
|---|---|---|---|
| every file as receipted | sound | verified | as now |
| the record another checksum, the masters as receipted | sound | verified | the record has changed at the archive since the deposit |
| a master another checksum | sound | differs | which master: not the one receipted |
| the record another checksum | not sound | differs | the archive's failures, as now |
| the record another checksum, on an archive with no fixity check | none: its receipt only | verified | the record has changed since; the receipt only |
| a file receipted is not held, or one is held that was not receipted | sound | differs | which file |
| the record changed since, the bag purged | sound | verified | the record has changed since |

Home now, item `1r` deposited under 0, the archive answering home 1:

| Where | Shows |
|---|---|
| Verify, the item's row | home 1, *deposited under 0*; the link to the site is under `/1/` |
| the photo's page | home 1 and the name the register gives 1, *deposited under 0* |
| the same, the archive answering home 0 | home 0, as now, nothing more |
| the same, the archive not to be asked | the home as sent, marked as such |
| the local bag, `batch.yaml`, the deposit answer | byte for byte as they were |

An open batch naming person 0, the register holding no 0:

| When | Then |
|---|---|
| New scan, Import, Feed or Import a film | refused: person 0 is not in the register; *Re-home* offered; nothing reserved |
| Re-home, person 1 chosen | `batch.yaml` names 1 and her name as the register gives it; scans may begin |
| Re-home, a photo packaged under 0 and not deposited | its bag is written again with home 1; its id the same |
| Re-home, a photo deposited under 0 | its bag, answer and record as they were |
| Re-home, no person chosen, or 0 | refused |
| Re-home on a batch whose person the register holds | not offered |
| Re-home on a finished batch | not offered: nothing more will be deposited from it |

Deposit, an id reserved under 0 and since moved by the archive to 1:

| The bag's home | The archive says the id is homed with | Then |
|---|---|---|
| 0 | 1 | not sent; refused: the batch names 0, the archive 1; *Re-home* offered |
| 1, after Re-home | 1 | sent and receipted |
| 1 | 1 | as now |
| 0 | the archive does not know the id | refused as now, nothing sent |

No id:

| Given | Then |
|---|---|
| 0 typed or sent as the home of a new batch | refused: 0 is no id |
| a `batch.yaml` with `person: "0"` | loads; listed with home *0, no longer an id* |
| a deposit answer `status: held`, `duplicate_of: "0"` | read as held, no duplicate |
| a deposit answer `status: duplicate`, `duplicate_of: "0"` | read as a duplicate of an item whose id is gone; Verify says *rejected* and names no item |
| an item held, receipted now | the answer written has no `duplicate_of` |
| the stores as they are on 2026-11-21 | every batch loads; Verify of batch 20261117-3 against UAT is *verified* for each item, the record changed since |

## Out of scope

- Rewriting local bags, `batch.yaml` or deposit answers to the new id:
  see *Decisions*.
- Purging the 41 bags early: purge takes them on its own day, the
  oldest on 2026-11-24.
- What the record now says: Verify says a record has changed, not how.
  bytestone-editor shows a record and its history.
- The store `~/.local/share/bytestone-scan` (the old production mirror,
  which holds the two open batches and items never pushed): what
  becomes of it is its own question; this story only makes it load and
  refuse rightly.
- life-weaver#309 itself. This side stops offering and writing 0; the
  archive makes it nobody.
- The scanner's own item ids and the workflow database: an item keeps
  its id through a person's renumber. Item 0's own move (life-weaver
  story 322) changes the id the local store knows it by, for the one
  item; Verify will say the archive does not know it.

## Decisions

No ADR expected. Accepting the story takes the choice in each row; say
otherwise to change one.

| Fork | Chosen | The other | Why |
|---|---|---|---|
| The local store after a renumber | Left as sent; the archive is asked for what is true now | A `renumber` of the local store, rewriting bags, batches and answers as life-weaver did its own | The bags are days from purge and are a copy of what was sent, which is what makes them worth checking against. A rewritten local bag would agree with the archive because it was made to. The cost is that an old batch goes on saying 0 |
| A record that differs at Verify | Verified, and said | A verdict of its own, *changed* | The question Verify answers is whether the archive holds what was scanned, and it does; the operator has nothing to do. A third verdict would want a colour and a count for a thing that is not a fault |
| An open batch naming a person who is gone | Re-homed by the operator | Refused outright: finish it and start another | Its scans and undeposited photos would be stranded, and choosing the person is one step |
| Where the home is checked before a deposit | In the scanner, by asking | In the archive, which could refuse a record whose home is not the id's | The archive does not read a photo's record (its rule since story 032), and this needs no release of it |

## Done when

- [x] tests express the acceptance tables and pass
- [x] spec updated (glossary, README); the format statement needs none: it says nothing of the local store
- [ ] ADR written if a fork was resolved
- [x] changelog fragment
- [ ] issue closed with a link to the release

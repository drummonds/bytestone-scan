# 109 — Verify looks at the site as well as the archive (#143)

**Status:** built 2026-11-03

## Why

Scan 2 of batch 20261027-1 verified: the archive held item 19 with every
checksum as receipted. Neither link under Published resolved, because
the last publish ran half an hour before the deposit. A page that says
*verified* in green while the item is not on the site answers the
wrong question: a photo is *held* by the archive and *published* on a
site, and the operator wants both facts, not one dressed as the other.

## What changes

- `internal/site`: a Site (name, base URL) can say whether it has an
  item's page, by a HEAD of `/<home>/items/<id>/`: published, not yet
  (404) or unreachable. The defaults are i.bytestone.uk and
  i.bytestone.org, in that order; `App.Sites` overrides them, so tests
  use fake sites.
- The verify page asks each site for every held item. The Published
  column tags each link with the answer. The notification adds a
  sentence per site, how many of the items it has, and is a warning
  rather than a success while any site lacks one; the help says what
  puts an item there (life-weaver's `task publish`, then `task promote`).

## Acceptance

| Archive | Every site has every held item | Notification | Published cell |
|---|---|---|---|
| verified | yes | success: verified, and on each site n of n | each link tagged *published* |
| verified | no | warning: verified; on *site* m of n | lacking link tagged *not yet* |
| verified | a site cannot be asked | warning | that link tagged *unreachable* |
| not verified | any | danger, as before, with the sites' sentence | as above |
| pending or rejected item | – | – | empty, as before |

## Out of scope

Checking what the site shows against the renditions in the bag. Asking
the site from the batch package: the site is the web layer's concern.

## Decisions

none

## Done when

- [x] tests express the acceptance table and pass

# 108 — Links to the published item on the verify page (#141)

**Status:** built 2026-11-03

## Why

Verifying a scan proves the archive holds each photo as receipted; the
next question is whether the photo is up on the site. The operator has to
work the address out by hand: the home person's id, then `items/`, then
the item id. The verify page knows both ids, so it can offer the links.

There are two sites. life-weaver's `task publish` builds and sends
i.bytestone.uk, where a publish is looked at; `task promote` then copies
that, as served, over i.bytestone.org, the production site. So a freshly
deposited photo is on i.bytestone.uk after the next publish and on
i.bytestone.org only after the promotion that follows; either link may
be a 404 until then.

## What changes

- `batch.Check` carries `Home`, the person the item sits under, from the
  deposit record.
- The verify page's table gains a *Published* column: for each deposited
  item, links to `https://i.bytestone.uk/<home>/items/<id>/` and
  `https://i.bytestone.org/<home>/items/<id>/`. Pending and rejected
  items have none.
- The page's help says the uk link works once life-weaver has published
  and the org link once it has promoted.

## Acceptance

| Deposit record | Published column |
|---|---|
| held (any verdict from the archive) | links to the item on i.bytestone.uk and i.bytestone.org |
| none (pending) | empty |
| duplicate (rejected) | empty; the detail names the holding item |

## Out of scope

Asking either site whether the page is there: the links are offered, not
checked. Links for the duplicate's holding item.

## Decisions

none

## Done when

- [x] tests express the acceptance table and pass

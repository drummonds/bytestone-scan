# 186 — The scanner has its own icon: a flatbed scanner, on its pages' tab (#186)

## Why

The scanner's tab shows lofigui's stock icon, the same as every other
lofigui tool, and life-weaver's status page is usually open beside it.
The user asked for an icon of its own, a scanner (2026-11-17).

## What changes

- `internal/web`: an icon, `icon.svg`, embedded and served: a flatbed
  scanner with its lid up over a picture, in life-weaver's blue and
  ochre on its off-white. Every page names it as its icon;
  `/favicon.ico`, which browsers ask for unprompted, serves the same
  picture.

## Acceptance

| Asked for | Outcome |
|---|---|
| `GET /assets/icon.svg` | the icon, as `image/svg+xml` |
| `GET /favicon.ico` | the same picture, as `image/svg+xml` |
| any page | its head names `assets/icon.svg` as the icon |

## Out of scope

- The documentation site's icon (bytestone-scan.docs.bytestone.uk).
- A desktop launcher or an app window.

## Decisions

None.

## Done when

- [ ] tests express the acceptance table and pass
- [ ] changelog fragment
- [ ] issue closed with a link to the release

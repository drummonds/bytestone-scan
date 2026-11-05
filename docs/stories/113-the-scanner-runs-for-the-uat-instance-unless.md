# 113 — The scanner runs for the UAT instance unless another is named (#152)

**Status:** built 2026-11-05

## Why

Since life-weaver story 026 nothing writes the production archive but
`task promote`: every depositor, the scanner included, writes UAT
(`bytestone-uat`, slot 1). The scanner still runs for production unless
told otherwise, so a plain `task serve` refuses to start whenever UAT is
serving alone, and the operator has to remember `-instance bytestone-uat`
every time. The navbar then shows the ordinary run in warning colours, as
if a scratch register were in use. The default should be where the
scanner's deposits now go.

## What changes

- `archive.Choose`: with no instance named, the UAT instance (purpose
  `uat`) is chosen among those serving. Production has to be named, as
  any other instance is. The refusal when no UAT instance is serving says
  what was found and how to start it.
- The navbar's archive tag: UAT in plain colours, as production was; any
  other instance, production now included, in warning colours with its
  slot, since the scanner on production is the odd case.
- `task serve` says it runs for UAT on `localhost:8081`; README, index.md
  and the glossary say UAT is the default and list `uat` among the
  purposes.

## Acceptance

| `-instance` | Serving on 8321–8330 | Outcome |
|---|---|---|
| none | bytestone (production) on 8321, bytestone-uat (uat, slot 1) on 8322 | UAT: archive 8322, listens on 8081, staging `<root>-bytestone-uat` |
| none | bytestone (production) only | refused: no UAT archive is serving; found bytestone on 8321 |
| none | none | refused: found none |
| bytestone | bytestone (production) on 8321 | production: archive 8321, listens on 8080, staging `<root>` |
| h3 | h3 (scale, slot 2) on 8323 | h3: archive 8323, listens on 8082, staging `<root>-h3` |
| geni | h3 (scale) only | refused naming geni and h3 (scale) |

| Instance the scanner is on | Navbar tag |
|---|---|
| bytestone-uat (uat) | plain: `archive bytestone-uat` |
| bytestone (production) | warning: `archive bytestone, slot 0` |
| example (scratch, slot 1) | warning: `archive example (scratch), slot 1` |
| not answering | danger: `archive not answering` |

## Out of scope

Refusing to run for production at all: the archive refuses the deposit,
and a named instance is a deliberate choice. The staging rule: production
keeps the plain root, every other instance `<root>-<name>`, UAT included,
so the batches already staged for UAT stay where they are.

## Decisions

- The default is the purpose, not the name: whichever instance says it is
  `uat`, so a renamed or second UAT tree needs no scanner change.

## Done when

- [x] tests express the acceptance table and pass
- [x] README, index.md, glossary updated; changelog fragment
- [ ] issue closed with a link to the release

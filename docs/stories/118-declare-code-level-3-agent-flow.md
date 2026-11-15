# 118 — Declare code level 3: agent flow (#162)

**Status:** proposed

## Why

At code level 1 a story reaches main only through `tp release` on its
branch, one release per story or a batch. The stories here are built by
an agent and reviewed against their acceptance table, which is what
task-plus's level 3 is for: a story comes from an idea, is defined
before it is built, and `tp story land` merges it once its review
passes. `tp story land` on story 117's branch was refused because the
project declares level 1.

## What changes

- `task-plus.yml`: `maturity.code: 3` (was `1`). The forge needs
  nothing: main on origin is already protected and merge commits are
  already the default.

## Acceptance

| Where | Command | Outcome |
|---|---|---|
| main, this story landed | `tp maturity` | `declared: code 3` |
| main, this story landed | `tp release query` | `Mode: trunk flow (maturity.code 3)` |
| a story branch | `tp story query` | answers with the story's next step, not the level-1 refusal |

## Out of scope

Story 116 (#157, import a film): its branch was cut at level 1 and is
brought under level 3 when it is picked up. `maturity.deploy` stays 0.

## Decisions

None.

## Done when

- [ ] `tp maturity` and `tp release query` on main say level 3
- [ ] changelog fragment
- [ ] issue closed with a link to the release

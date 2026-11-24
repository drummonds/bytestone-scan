# 206 — Spikes are not kept in the repo (#206)

## Why

A spike is throwaway code under `spike/`. Untracked, it makes the tree
dirty and blocks a release: `spike/reading` did on 2026-11-18, and
story 193 answered by keeping it in the repo; `spike/pages` did again
on 2026-11-22, before v0.40.0. Keeping each spike is a story apiece for
code that is not built, checked or released. What lasts of a spike is
its note, under `docs/research`, or the story it led to.

## What changes

- `.gitignore`: `/spike/`. A spike is written there and stays on the
  machine it was run on.
- `spike/reading/` leaves the index (`git rm --cached`): its six files
  stay on disk here, untracked and ignored, and in history as of
  v0.40.0.
- `docs/research/2026-11-18-reading-letters-with-claude.md`: where it
  points at `spike/reading/`, it says the code is in the repo's history
  at v0.40.0.

## Acceptance

| Where | Asked for | Outcome |
|---|---|---|
| repo root, a new file under `spike/` | `git status` | clean: the file is ignored |
| the repo | `git ls-files spike` | nothing |
| this machine | `spike/reading/`, `spike/pages/` on disk | as they were |
| the repo | `git show v0.40.0:spike/reading/main.go` | the spike, as story 193 kept it |
| repo root | `task check` | passes |
| the research note | where the spike's code is | history at v0.40.0, not the tree |

## Out of scope

- Rewriting history to take the spike out of it.
- `spike/pages`: never tracked; story 202 names it and stands.
- The research note's findings, and story 193's text: a record of what
  was done then.

## Decisions

No ADR. Accepting the story takes the choice in the row; say otherwise
to change it.

| Fork | Chosen | The other | Why |
|---|---|---|---|
| `spike/reading`, which story 193 kept as the record of how the note's figures were made | Out of the index; history at v0.40.0 is the record | Keep it tracked and ignore only new spikes | One rule for `spike/`. The cost is that a fresh clone no longer has the code, the reference reading, facts and hints for `2j` in its tree, and must take them from the tag |

## Done when

- [x] acceptance table checked by hand (git configuration: no tests)
- [x] spec updated (the research note)
- [x] changelog fragment
- [ ] issue closed with a link to the release

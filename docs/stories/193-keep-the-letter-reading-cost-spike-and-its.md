# 193 — Keep the letter-reading cost spike and its research note in the repo (#193)

## Why

On 2026-11-18 a spike measured what it costs to have a Claude model
transcribe a scanned letter and propose its people, dates and events,
across four models, on item `2j`. Its code, its reference reading and
the note of its results sat untracked on main, where they block a
release (the tree is dirty) and could be lost. The stories on reading
letters, not yet written, will start from this note.

## What changes

- `docs/research/2026-11-18-reading-letters-with-claude.md`: the note,
  as written: question, method, results, limits.
- `spike/reading/`: the spike, a Go module of its own, with its
  reference reading, facts and hints for item `2j`. Throwaway code,
  kept as the record of how the note's figures were made; it has no
  tests and is not built, checked or released with the scanner.
- `.gitignore`: `.tp-release-inflight` moves a line down, as tp left it.

## Acceptance

| Where | Asked for | Outcome |
|---|---|---|
| repo root | `task check` | passes; it does not format, vet or test `spike/reading` |
| repo root | the scanner's `go.mod` | unchanged: no Anthropic SDK among its requirements |
| `spike/reading` | `go vet .` | clean |
| `spike/reading` | `gofmt -l .` | names no file |
| the repo | images | none added; the spike's readings stay under `staging/` |

## Out of scope

- Reading letters in the scanner: no story yet; the note is its input.
- A corrected reference, more letters, repeat runs, a choice of model.
- Rendering `docs/research` on the documentation site.

## Decisions

None.

## Done when

- [x] acceptance table checked by hand (a spike: no tests)
- [x] changelog fragment
- [ ] issue closed with a link to the release

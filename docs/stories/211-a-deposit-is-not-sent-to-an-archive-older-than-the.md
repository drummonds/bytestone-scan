# 211 — A deposit is not sent to an archive older than the scanner is built for, and a failed item is named by its kind (#211)

## Why

On 2026-11-25 a writing, item 2p, was imported with the scanner at
v0.41.0 and deposited with UAT, whose service was still life-weaver
v0.58.0. That release does not know `data/writing.yaml` (the kind came
in v0.61.0), took the bag for a photo's and refused it for having no
renditions. The scanner then said *Photo 2p failed at deposit: refused
for its renditions … the renditions were remade at recipe 3, Retry
deposits again*.

Three things in that are wrong. The cause, an archive behind the
scanner, is nowhere said, so the operator is sent to look at renditions.
The item is a writing and is called a photo. And a writing has no
renditions: none were remade, and Retry could only fail again.

## What changes

- `internal/archive`: the client asks the service which life-weaver
  release it serves (`GET /version`, which it reads already for the
  instance).
- `internal/batch`: before an item's deposit is sent, the release the
  archive serves is compared with the life-weaver release the scanner is
  built against (the `git.bytestone.uk/hum3/life-weaver` version in its
  build, the one `go.mod` names). An older archive is not sent the
  deposit; the refusal names both releases and says what to do.
- `internal/batch`: a scan's standing names the item holding it back by
  its kind, *writing 2p*, *film 2p*, *letter 2p*, where it said *photo*
  for every kind.
- `internal/batch`: a writing's deposit refused for its renditions says
  nothing of remaking them or of Retry.

## Acceptance

### An item's deposit, by the release the archive serves

The scanner here is built against life-weaver v0.61.0.

| The archive serves | Outcome |
|---|---|
| `v0.58.0`, older | nothing is sent. The item fails at deposit: *not sent: the archive at URL serves life-weaver v0.58.0, older than the v0.61.0 this scanner is built for; deploy v0.61.0 or later to it, then Retry* |
| `v0.61.0`, the same | deposited, as now |
| `v0.62.0` or `v1.0.0`, newer | deposited, as now |
| `v0.60.9` against `v0.61.0`; `v0.9.0` against `v0.10.0` | older: compared as numbers, part by part, not as text |
| `(devel)`, a local build, or anything else that is not a release | deposited, as now: what cannot be compared is not refused |
| nothing: the service does not answer | fails as a deposit to a service not answering does now |
| `v0.58.0`, then `v0.61.0` deployed and Retry pressed | deposited |

The scanner's own build not saying which life-weaver it holds (a test
binary) is the same as the archive's not saying: nothing is refused.

### How a scan's standing names the item holding it back

| The scan is | Its item 2p failed at deposit |
|---|---|
| a scan of prints | *photo 2p failed at deposit: …*, as now |
| a feed of letters | *letter 2p failed at deposit: …* |
| a film import | *film 2p failed at deposit: …* |
| a writing import | *writing 2p failed at deposit: …* |

The same name is used for every status the standing gives an item
(*writing 2p cancelled*, *writing 2p waiting at describe*).

### A deposit the archive refuses for its renditions

| The item is | Outcome |
|---|---|
| a photo, a letter, a film | its renditions are remade and the failure ends *the renditions were remade at recipe N, Retry deposits again*, as now |
| a writing | the archive's refusal alone: nothing is remade, and nothing is said of renditions remade or of Retry |

## Out of scope

- Checking the archive's release anywhere but at deposit: not when the
  scanner starts, not when a batch or an import is begun, and no mark in
  the navigation bar. The deposit is where an old archive does harm, and
  Retry is there once it is deployed.
- A release needed per kind (a photo could still go to v0.58.0). One
  rule for every kind: the archive is not behind the scanner.
- Deploying the archive, and any change to life-weaver.
- Other wording that says *photo* of an item of another kind, on pages
  or in other refusals: this story changes the standing only.

## Decisions

- The release needed is read from the build, not written in the code as
  a constant: `go.mod` already says which archive the client is built
  to, and a constant would be a second statement of it to forget. The
  cost is that raising the dependency for any reason asks for the
  archive to be deployed before the next deposit. No ADR.

## Done when

- [ ] tests express the acceptance tables and pass
- [ ] changelog fragment
- [ ] issue closed with a link to the release

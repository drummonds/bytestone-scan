# 196 — `task serve` scans for the hosted UAT archive, and `-archive` takes port and staging from the instance it finds there (#196)

## Why

Since life-weaver story 188 the archive service runs on the NAS over
UAT, always up, at `https://bytestone-uat.lan.drummonds.net`, so that no
one starts a service to scan. The scanner still looks only on
`localhost:8321-8330`: `task serve` refuses with *no UAT archive is
serving* and tells the operator to start one in life-weaver (2026-11-22).
bytestone-editor's `task serve` already names the hosted service.

Naming it by hand with `-archive` is unsafe as it stands: an `-archive`
given is taken as it is, no instance asked for, so the scanner listens
on `localhost:8080` and stages under `~/.local/share/bytestone-scan`,
production's staging, whatever instance the URL serves.

## What changes

- `bytestone-scan serve` and `verify`, `-archive URL`: the service at
  the URL is asked which instance it serves, and the scanner's address
  and staging root follow from that instance exactly as they do for one
  found on localhost. A service that does not answer, or does not say
  which instance it is, is refused. `-instance` beside `-archive` must
  name the instance found there.
- The refusal when no instance is found on localhost also says a service
  elsewhere is named with `-archive URL`.
- `Taskfile.yml`: `task serve` runs the scanner for the hosted service,
  `-archive https://bytestone-uat.lan.drummonds.net`; `task serve:local`
  (new) is today's `task serve`, an instance found on localhost
  (`-- -instance NAME` for another than UAT).
- README, `index.md` and the glossary's *Archive instance*: where the
  instance is found.

## Acceptance

`root` is `~/.local/share/bytestone-scan`. *At the URL* is what the
service named by `-archive` answers.

### Settings from `-archive`

| `-archive` | At the URL | `-instance` | `-addr`, `-staging` | Outcome |
|---|---|---|---|---|
| given | `bytestone-uat`, uat, slot 1 | absent | absent | that URL; `localhost:8081`; `root-bytestone-uat` |
| given | `bytestone`, production, slot 0 | absent | absent | that URL; `localhost:8080`; `root` |
| given | `h3`, scale, slot 2 | absent | absent | that URL; `localhost:8082`; `root-h3` |
| given | `bytestone-uat`, uat, slot 1 | `bytestone-uat` | absent | as the first row |
| given | `bytestone-uat`, uat, slot 1 | `h3` | absent | refused: names the URL, `h3` and `bytestone-uat` |
| given | `bytestone-uat`, uat, slot 1 | absent | both given | that URL; the address and staging given |
| given | no answer | any | any | refused: names the URL, says it is not answering |
| given | answers, names no instance | any | any | refused: names the URL, says it does not say which instance it serves |
| absent | (not asked) | any | any | as now: found on `localhost:8321-8330` |

### The refusal on localhost

| `-instance` | Found on localhost | Outcome |
|---|---|---|
| absent | no UAT instance | refused as now, and says a service elsewhere is named with `-archive URL` |
| `geni` | not `geni` | refused as now, and says the same |

### Tasks

| Task | Outcome |
|---|---|
| `task serve` | the scanner for the service at `https://bytestone-uat.lan.drummonds.net`: on `localhost:8081`, staging `root-bytestone-uat` |
| `task serve -- -recorded demo` | the same, on the recording |
| `task serve:local` | the scanner for the UAT instance found on localhost |
| `task serve:local -- -instance h3` | the scanner for `h3` found on localhost |

## Out of scope

- The installed command's default: `bytestone-scan serve` with no
  `-archive` still looks on localhost. The hosted name is the Taskfile's,
  not the program's.
- A configuration file shared with bytestone-editor, bytestone-loader
  and life-weaver. One URL in two Taskfiles does not earn it; a loader
  that deposits through the service would.
- bytestone-loader, which writes the laptop's mirror as a tree and does
  not use the service.
- The scanner run on a machine off the LAN.

## Decisions

None.

## Done when

- [x] tests express the acceptance tables and pass (the tasks by hand)
- [x] README, `index.md` and glossary updated
- [x] changelog fragment
- [ ] issue closed with a link to the release

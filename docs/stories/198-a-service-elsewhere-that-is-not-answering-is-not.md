# 198 — A service elsewhere that is not answering is not said to be started on this machine, and `task serve` says UAT is on the NAS (#198)

## Why

The review of story 196 left two wordings wrong. When the archive
service does not answer, the refusal advises *start it with life-weaver
serve, or task serve in life-weaver*; since 196 the service is usually
the hosted one, on the NAS, which nothing on the laptop starts. And
`task serve`'s description says *UAT behind hydrogen*, a host the README
and the story do not name: they say the NAS.

## What changes

- `internal/archive`: the not-answering refusal advises starting the
  service only when its URL is on this machine; for a service elsewhere
  it says to check that it is up and can be reached from here.
- `Taskfile.yml`: `task serve`'s description says the hosted archive
  service, UAT on the NAS.

## Acceptance

### The archive service is not answering

| The service's URL | Outcome |
|---|---|
| on this machine (`localhost`, `127.0.0.1`, `[::1]`) | names the URL, says it is not answering and how to start it (`life-weaver serve`, or `task serve` in life-weaver), as now |
| elsewhere (`https://bytestone-uat.lan.drummonds.net`) | names the URL, says it is not answering and to check it is up and reachable from this machine; says nothing of starting it with life-weaver |

### `task --list`

| Task | Outcome |
|---|---|
| `serve` | says the hosted archive service, UAT on the NAS; does not name hydrogen |

## Out of scope

- Any other wording, and any behaviour: what is refused and when is as
  story 196 left it.

## Decisions

None.

## Done when

- [x] tests express the first table and pass (the task's description by hand)
- [x] changelog fragment
- [ ] issue closed with a link to the release

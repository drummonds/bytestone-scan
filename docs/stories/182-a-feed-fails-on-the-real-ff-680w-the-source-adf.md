# 182 — A feed fails on the real FF-680W: the source ADF Duplex is sent to scanimage as two arguments (#182)

## Why

On 2026-11-17, with v0.36.1, the first *Scan letters* on the real
FF-680W failed at its first step:

    feed: exit status 1: scanimage: setting of option --source failed (Invalid argument)

The feeder's fixed settings are kept as the lines recorded with a
master, `--source ADF Duplex` among them, and each line is split at
every space to make the command. `scanimage` is given `--source ADF`
and a stray `Duplex`; the scanner has no source `ADF`. The tests'
stand-in takes any arguments, and the test reads them joined by spaces,
where the right and the wrong command look the same.

## What changes

- `internal/fastfoto`: each setting is sent as its option and one
  value, whatever spaces the value has.
- `internal/fastfoto/testdata/scanimage`: the stand-in refuses a
  `--source` that is not one of the FF-680W's, with `scanimage`'s own
  message, as the real one does.

## Acceptance

| The feed's command | Outcome |
|---|---|
| `--source` and `ADF Duplex`, one argument each | the stand-in feeds; the sides are handed over |
| `--source ADF` then `Duplex` (as sent until now) | the stand-in refuses: "setting of option --source failed (Invalid argument)", and the feed fails with it |
| the other settings (`--mode Color`, `--depth 8`, `--adf-skew=no`) | sent as before |
| the settings recorded with a master | as before: `--source ADF Duplex` as one line |

## Out of scope

- The 4990's settings (`internal/epson`): none of its values has a
  space.

## Decisions

None.

## Done when

- [ ] tests express the acceptance table and pass
- [ ] changelog fragment
- [ ] a real sheet fed through *Scan letters*
- [ ] issue closed with a link to the release

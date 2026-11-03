# 110 — Cancel a scan, which leaves it out of the batch (#145)

**Status:** built 2026-11-03

## Why

Scan 2 of batch 20261103-1 previewed an empty bed: no prints were
found, so it waits at confirm with nothing to confirm, and the batch's
Finish scanning waits for it. The only way out is to delete the whole
batch, or to cancel the scan's run from its workflow instance page, which
leaves the batch waiting on a scan that reads *cancelled*. A scan that
was a mistake, a false start or an empty bed should be abandoned from its
own page, and the batch should carry on without it.

## What changes

- `batch.Scan.Cancel`: cancels the scan's run while that run is not
  through; a scan through to its photos, or with no run, is refused.
- A cancelled scan is left out of its batch: `Batch.Finish` and the
  batch's scans step do not wait for it, and a batch whose scans are all
  cancelled has nothing scanned. `Standing.Cancelled` says which.
- The scan page: a Cancel this scan button beside the step's buttons at
  every wait and on a failure, none while a step is running (Stop first)
  and none once the scan is through. A cancelled scan says so, with no
  step to carry on; the batch page's row says *cancelled* and Finish
  scanning does not list it.
- `POST /batches/{id}/scans/{n}/cancel`.

## Acceptance

| Scan's run | A step running | Cancel | Scan page | Batch |
|---|---|---|---|---|
| waiting, at any wait | no | cancelled | *Cancelled: left out of the batch*, no buttons | Finish scanning ignores the scan |
| failed, or stopped | no | cancelled | as above | as above |
| any | yes | refused, 409: stop the step first | Stop, as now; no Cancel | – |
| completed | – | refused, 422 | no Cancel | – |
| cancelled | – | refused, 422 | as above | left out |
| none | – | refused, 422 | – | – |
| every scan cancelled | – | – | – | Finish scanning refused: nothing has been scanned |

The archive never holds a photo of a cancelled scan: photos are
deposited only once the scan is through, and a cancelled run is not run
again. Masters already captured stay in staging until the batch is
purged, as a stopped scan's do.

## Out of scope

Reviving a cancelled scan: do one more scan. Deleting its files before
the purge. Cancelling a batch (delete it).

## Decisions

none

## Done when

- [x] tests express the acceptance table and pass
- [x] changelog entry
- [ ] issue closed with a link to the release

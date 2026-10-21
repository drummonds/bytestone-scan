package batch

import wf "git.bytestone.uk/hum3/gobank-workflow"

// Workflow is the batch's workflow type.
const Workflow wf.WorkflowType = "scan_batch"

const src = "https://git.bytestone.uk/hum3/bytestone-scan/src/branch/main/internal/batch/batch.go"

func init() {
	wf.Definitions = append(wf.Definitions, wf.Definition{
		Type: Workflow,
		Purpose: wf.Purpose{
			Name:        "Archive photographs",
			Description: "Capture prints as archival masters, each in its own bag, under the person they belong to.",
		},
		Description: "Scans one load of the bed: finds the prints on a preview, waits for the operator to confirm them, " +
			"scans them in one detail pass, waits for the masters to be accepted and bags each. " +
			"A retry resumes at the first step not done; the operator can send the batch back to any step before packaging.",
		InstanceKey: "batch id",
		Steps: []wf.Step{
			{Name: "describe", Description: "The batch's description, home person and scan ppi are recorded", Source: src},
			{Name: "preview", Description: "The whole bed is scanned at 150 ppi", Source: src},
			{Name: "find prints", Description: "Prints are found on the preview, with a margin; doubtful ones are flagged for review", Source: src},
			{Name: "confirm", Description: "Waits for the operator to accept the prints, then reserves a photo id for each", Source: src},
			{Name: "detail pass", Description: "One scan over every print, averaged down to each master's ppi and written as master TIFFs", Source: src},
			{Name: "check quality", Description: "Waits for the operator to accept the masters, or to send the batch back a step", Source: src},
			{Name: "package", Description: "Each master and its photo.yaml become a bag; final, since a bag is never altered", Source: src},
		},
	})
}

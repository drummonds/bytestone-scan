package batch

import wf "git.bytestone.uk/hum3/gobank-workflow"

// Workflow is the batch's workflow type.
const Workflow wf.WorkflowType = "scan_batch"

const src = "https://git.bytestone.uk/hum3/bytestone-scan/src/branch/main/internal/batch/batch.go"

// Definitions are the workflows the scanner runs: what its Workflows page
// lists and draws. The engine's own registry is seeded with other hosts'
// workflows, so the scanner never hands that to the view.
var Definitions = []wf.Definition{
	{
		Type: Workflow,
		Purpose: wf.Purpose{
			Name:        "Archive photographs",
			Description: "Capture prints as archival masters, each archived as a photo in the collection of the person it belongs to.",
		},
		Description: "Scans one load of the bed: finds the prints on a preview, waits for the operator to confirm them, " +
			"scans them in one detail pass, develops each master's renditions, waits for the masters to be accepted and archives each. " +
			"A retry resumes at the first step not done; the operator can send the batch back to any step before packaging.",
		InstanceKey: "batch id",
		Steps: []wf.Step{
			{Name: "describe", Description: "The batch's description, home person and scan ppi are recorded", Source: src},
			{Name: "preview", Description: "The whole bed is scanned at 150 ppi", Source: src},
			{Name: "find prints", Description: "Prints are found on the preview, with a margin; doubtful ones are flagged for review", Source: src},
			{Name: "confirm", Description: "Waits for the operator to accept the prints, then reserves a photo id for each", Source: src},
			{Name: "detail pass", Description: "One scan over every print, averaged down to each master's ppi and written as master TIFFs", Source: src},
			{Name: "develop", Description: "Each master's renditions are made: the print measured on the master, cropped, turned square, its levels set, and scaled to fit a screen", Source: src},
			{Name: "check quality", Description: "Waits for the operator to accept the masters, or to send the batch back a step", Source: src},
			{Name: "package", Description: "Each master and its photo.yaml are archived as a photo; final, since an archived photo is never altered", Source: src},
		},
	},
}

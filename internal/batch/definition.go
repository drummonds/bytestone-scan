package batch

import wf "git.bytestone.uk/hum3/gobank-workflow"

// Workflow is the batch's workflow type; LoadWorkflow is each load's, run
// as a child of its batch's; PhotoWorkflow is each photo's, under its load's.
const (
	Workflow      wf.WorkflowType = "scan_batch"
	LoadWorkflow  wf.WorkflowType = "scan_load"
	PhotoWorkflow wf.WorkflowType = "scan_photo"
)

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
		Description: "One sitting of scanning: one home person, provenance, credit and scan ppi, over as many loads of the bed as the shoebox needs. " +
			"Each load is a workflow of its own under the batch; the batch waits until the operator finishes scanning and every load is through.",
		InstanceKey: "batch id",
		Steps: []wf.Step{
			{Name: "describe", Description: "The batch's title, home person, provenance, credit and scan ppi are recorded", Source: src},
			{Name: "loads", Description: "A load of the bed at a time, each started by the operator; waits until scanning is finished and every load is through", Source: src, Child: LoadWorkflow, Many: true},
		},
	},
	{
		Type: LoadWorkflow,
		Purpose: wf.Purpose{
			Name:        "Scan a load of the bed",
			Description: "Capture the prints on the bed as archival masters, each archived as a photo of its batch's home person.",
		},
		Description: "Scans one load of the bed: finds the prints on a preview, waits for the operator to confirm them, " +
			"scans the fronts in one pass and starts a photo workflow for each, waits for the prints to be turned over and scans the backs where the fronts lay, " +
			"then waits for the masters to be accepted. A retry resumes at the first step not done; the operator can send the load back to any step until its photos are archived.",
		InstanceKey: "batch id/load",
		Steps: []wf.Step{
			{Name: "describe", Description: "The load's description is recorded, pre-filled from its batch's title", Source: src},
			{Name: "preview", Description: "The whole bed is scanned at 150 ppi", Source: src},
			{Name: "find prints", Description: "Prints are found on the preview, with a margin; doubtful ones are flagged for review", Source: src},
			{Name: "confirm", Description: "Waits for the operator to accept the prints, then reserves a photo id for each", Source: src},
			{Name: "fronts", Description: "One scan over every print's front, averaged down to each master's ppi and written as 16-bit master TIFFs", Source: src},
			{Name: "photos", Description: "A photo workflow is started for each print", Source: src, Child: PhotoWorkflow, Many: true},
			{Name: "backs", Description: "Waits for the operator to turn the prints over in place, or to say the backs are blank; then previews the bed to see where each back lies and scans the fronts' regions once more at 300 ppi, 8 bits; a back whose size differs from its front's is flagged for review", Source: src},
			{Name: "check quality", Description: "Waits for the operator to accept the masters, or to send the load back a step", Source: src},
		},
	},
	{
		Type: PhotoWorkflow,
		Purpose: wf.Purpose{
			Name:        "Archive one photo",
			Description: "Develop each side of one print as it is scanned and archive both as a photo.",
		},
		Description: "One print of a load: its front is developed as soon as it is scanned, it waits for its back, develops that too, " +
			"waits for the load's masters to be accepted, and is archived with its photo.yaml. Final once archived, since an archived photo is never altered.",
		InstanceKey: "photo id",
		Steps: []wf.Step{
			{Name: "develop front", Description: "The front's renditions are made: the print measured on the master, cropped, turned square, its levels set, and scaled to fit a screen", Source: src},
			{Name: "awaiting back", Description: "Waits for the load's backs pass to have scanned this print's back", Source: src},
			{Name: "develop back", Description: "The back's renditions are made the same way", Source: src},
			{Name: "accepted", Description: "Waits for the operator to accept the load's masters", Source: src},
			{Name: "describe", Description: "Waits for the operator to say what is known of the photo: the date taken, the photographer and what the back says, each a claim in photo.yaml with its source; nothing known is an answer", Source: src},
			{Name: "package", Description: "Both masters and the photo.yaml are archived as a photo; final, since an archived photo is never altered", Source: src},
		},
	},
}

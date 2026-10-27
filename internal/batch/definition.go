package batch

import wf "git.bytestone.uk/hum3/gobank-workflow"

// Workflow is the batch's workflow type; ScanWorkflow is each scan's, run
// as a child of its batch's; PhotoWorkflow is each photo's, under its scan's.
const (
	Workflow      wf.WorkflowType = "scan_batch"
	ScanWorkflow  wf.WorkflowType = "scan"
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
		Description: "One sitting of scanning: one home person, provenance, credit and scan ppi, over as many scans of the bed as the shoebox needs. " +
			"Each scan is a workflow of its own under the batch; the batch waits until the operator finishes scanning and every scan is through, then closes.",
		InstanceKey: "batch id",
		Steps: []wf.Step{
			{Name: "describe", Description: "The batch's title, home person, provenance, credit and scan ppi are recorded", Source: src},
			{Name: "scans", Description: "A scan of the bed at a time, each started by the operator; waits until scanning is finished and every scan is through: every photo held by the archive or rejected by it", Source: src, Child: ScanWorkflow, Many: true},
			{Name: "close", Description: "The batch closes: its work files are redundant, and are purged 30 days later, or sooner when staging is short of space", Source: src},
		},
	},
	{
		Type: ScanWorkflow,
		Purpose: wf.Purpose{
			Name:        "Scan the bed once",
			Description: "Capture the prints on the bed as archival masters, each archived as a photo of its batch's home person.",
		},
		Description: "One placing of prints on the bed: finds the prints on a preview, waits for the operator to confirm them, " +
			"scans the fronts in one pass and starts a photo workflow for each, waits for the prints to be turned over and scans the backs where the fronts lay, " +
			"then waits for the masters to be accepted. A retry resumes at the first step not done; the operator can send the scan back to any step until its photos are archived.",
		InstanceKey: "batch id/scan",
		Steps: []wf.Step{
			{Name: "describe", Description: "The scan's description is recorded, pre-filled from its batch's title", Source: src},
			{Name: "preview", Description: "The whole bed is scanned at 150 ppi", Source: src},
			{Name: "find prints", Description: "Prints are found on the preview, with a margin; doubtful ones are flagged for review", Source: src},
			{Name: "confirm", Description: "Waits for the operator to accept the prints, then reserves a photo id for each", Source: src},
			{Name: "fronts", Description: "One scan over every print's front, averaged down to each master's ppi and written as 16-bit master TIFFs", Source: src},
			{Name: "photos", Description: "A photo workflow is started for each print", Source: src, Child: PhotoWorkflow, Many: true},
			{Name: "backs preview", Description: "Waits for the operator to turn the prints over in place, or to say the backs are blank; then previews the bed to see where each back lies within its front's region; a back whose size differs from its front's, or that is not found, is flagged for review", Source: src},
			{Name: "backs", Description: "Waits for the operator to confirm the backs on their preview, then scans the fronts' regions once more at 300 ppi, 8 bits", Source: src},
			{Name: "check quality", Description: "Waits for the operator to accept the masters, or to send the scan back a step", Source: src},
			{Name: "page", Description: "An import whose page holds several prints: the page is bagged as an item of its own, the file as sent with page.yaml, and its renditions made; final", Source: src},
			{Name: "deposit page", Description: "The page's bag and renditions are handed to the archive service: held, or a duplicate of a page it holds already, which the photos are then cut from", Source: src},
			{Name: "page receipt", Description: "The archive's record of the page is read and checked against the bag", Source: src},
		},
	},
	{
		Type: PhotoWorkflow,
		Purpose: wf.Purpose{
			Name:        "Archive one photo",
			Description: "Develop each side of one print as it is scanned and archive both as a photo.",
		},
		Description: "One print of a scan: its front is developed as soon as it is scanned, it waits for its back, develops that too, " +
			"waits for the scan's masters to be accepted and for its description, is bagged with its photo.yaml, deposited with the archive and receipted. " +
			"Final once bagged, since a bagged photo is never altered.",
		InstanceKey: "photo id",
		Steps: []wf.Step{
			{Name: "develop front", Description: "The front's renditions are made: the print measured on the master, cropped, turned square, its levels set, and scaled to fit a screen", Source: src},
			{Name: "awaiting back", Description: "Waits for the scan's backs pass to have scanned this print's back", Source: src},
			{Name: "develop back", Description: "The back's renditions are made the same way", Source: src},
			{Name: "accepted", Description: "Waits for the operator to accept the scan's masters", Source: src},
			{Name: "describe", Description: "Waits for the operator to say what is known of the photo: the date taken, the photographer and what the back says, each a claim in photo.yaml with its source; nothing known is an answer", Source: src},
			{Name: "page held", Description: "Waits for the page the photo was cut from, when the page is an item of its own, to be deposited", Source: src},
			{Name: "package", Description: "Both masters and the photo.yaml are bagged as a photo; final, since a bagged photo is never altered", Source: src},
			{Name: "deposit", Description: "The bag and its renditions at the current recipe are handed to the archive service; held, or refused as a duplicate of an item it holds already, which ends the photo, or refused for the renditions, which are made again and the deposit tried again", Source: src},
			{Name: "receipt", Description: "The archive's record of the item is read and checked against the bag, checksum for checksum", Source: src},
		},
	},
}

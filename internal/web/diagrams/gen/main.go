// Command gen draws each workflow the scanner runs twice, from the same
// registry the engine runs, rendered by the d2 CLI: diagrams/<type>.svg
// across in one row for a wide screen and diagrams/<type>.down.svg down
// in one column for a phone, which the view picks between. The pictures
// are committed so that the app embeds them without d2 at hand; run
// `task diagrams` after changing a definition.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	wf "git.bytestone.uk/hum3/gobank-workflow"
	"git.bytestone.uk/hum3/gobank-workflow/diagram"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
)

func main() {
	out := flag.String("out", "internal/web/diagrams", "directory the pictures are written to")
	flag.Parse()
	// A sub-process box links to its child's page, at the view's address.
	href := func(t wf.WorkflowType) string { return "workflows/definitions/" + string(t) }
	ways := []struct {
		dir    diagram.Direction
		suffix string
	}{{diagram.Across, ""}, {diagram.Down, ".down"}}
	for _, d := range batch.Definitions {
		for _, way := range ways {
			path := filepath.Join(*out, string(d.Type)+way.suffix+".svg")
			cmd := exec.Command("d2", "-", path)
			cmd.Stdin = strings.NewReader(diagram.Definition(d, way.dir, href))
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				log.Fatalf("d2 %s %s: %v", d.Type, way.dir, err)
			}
			fmt.Println(path)
		}
	}
}

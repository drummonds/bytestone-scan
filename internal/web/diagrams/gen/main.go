// Command gen draws each workflow the scanner runs as diagrams/<type>.svg,
// from the same registry the engine runs, rendered by the d2 CLI. The
// pictures are committed so that the app embeds them without d2 at hand;
// run `task diagrams` after changing a definition.
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
	for _, d := range batch.Definitions {
		path := filepath.Join(*out, string(d.Type)+".svg")
		cmd := exec.Command("d2", "-", path)
		cmd.Stdin = strings.NewReader(diagram.Definition(d, href))
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			log.Fatalf("d2 %s: %v", d.Type, err)
		}
		fmt.Println(path)
	}
}

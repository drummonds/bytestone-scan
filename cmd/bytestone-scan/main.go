// Command bytestone-scan scans batches of prints on the Epson 4990 into
// archival masters, one bag per photo.
//
//	bytestone-scan batch -describe "SS Bonita" -person 0 -ppi 2400
//	bytestone-scan confirm <batch>
//	bytestone-scan resume <batch>
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"image"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	_ "git.bytestone.uk/hum3/go-postgres"
	wf "git.bytestone.uk/hum3/gobank-workflow"
	"git.bytestone.uk/hum3/gobank-workflow/sqlstore"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
	"git.bytestone.uk/hum3/bytestone-scan/internal/epson"
	"git.bytestone.uk/hum3/bytestone-scan/internal/find"
	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
	"git.bytestone.uk/hum3/bytestone-scan/internal/register"
)

const usage = `usage:
  bytestone-scan batch -describe TEXT -person ID -ppi 600|1200|2400 [flags]
  bytestone-scan confirm BATCH [flags]   accept the prints found, then scan them
  bytestone-scan resume BATCH [flags]    carry on after a failure

flags:
`

type config struct {
	staging, people, device string
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1], os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "bytestone-scan:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cmd string, args []string) error {
	home, _ := os.UserHomeDir()
	var c config
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage); fs.PrintDefaults() }
	fs.StringVar(&c.staging, "staging", filepath.Join(home, ".local/share/bytestone-scan"), "staging root: batches, bags and the workflow database")
	fs.StringVar(&c.people, "people", filepath.Join(home, "git/hum3/life-weaver/data/people"), "life-weaver's register of people")
	fs.StringVar(&c.device, "device", "", "SANE device; found from scanimage -L if empty")
	describe := fs.String("describe", "", "batch: what is being scanned")
	person := fs.String("person", "", "batch: id of the person whose album the photos belong to")
	ppi := fs.Int("ppi", 1200, "batch: scan ppi, 600, 1200 or 2400")

	var batchID string
	switch cmd {
	case "batch":
		fs.Parse(args)
	case "confirm", "resume":
		if len(args) == 0 {
			fs.Usage()
			os.Exit(2)
		}
		batchID = args[0]
		fs.Parse(args[1:])
	default:
		fs.Usage()
		os.Exit(2)
	}

	var b *batch.Batch
	var err error
	if cmd == "batch" {
		id, err := pid.Parse(*person)
		if err != nil {
			return fmt.Errorf("-person: %w", err)
		}
		b, err = batch.Describe(c.staging, register.Register{Dir: c.people},
			batch.Description{Text: *describe, Person: id, ScanPPI: *ppi}, time.Now())
		if err != nil {
			return err
		}
		fmt.Printf("batch %s: %s, for %s (%s), at %d ppi\n", b.ID, b.Description, b.PersonName, b.Person, b.ScanPPI)
	} else if b, err = batch.Open(c.staging, batchID); err != nil {
		return err
	}
	if cmd == "confirm" {
		if err := b.Confirm(); err != nil {
			return err
		}
	}

	db, err := sql.Open("pglike", filepath.Join(c.staging, "workflow.db"))
	if err != nil {
		return err
	}
	defer db.Close()
	store, err := sqlstore.New(db)
	if err != nil {
		return err
	}
	scanner := &telling{&epson.Scanner{Device: c.device}}
	rec, err := b.Run(ctx, wf.NewRunner(store, nil), scanner, time.Now)
	if err != nil {
		return fmt.Errorf("%w\nonce fixed: bytestone-scan resume %s", err, b.ID)
	}
	if rec == nil || rec.Status == wf.StatusCompleted {
		return report(b, true)
	}
	return report(b, false)
}

// report says what the operator has to do next.
func report(b *batch.Batch, done bool) error {
	prints, ids, err := b.Found()
	if err != nil {
		return err
	}
	for i, p := range prints {
		line := fmt.Sprintf("  %d: %.0f × %.0f mm at %.0f, %.0f mm", i+1, p.Print.X1-p.Print.X0, p.Print.Y1-p.Print.Y0, p.Print.X0, p.Print.Y0)
		if i < len(ids) {
			line += ", photo " + ids[i].String()
		}
		if p.Review {
			line += fmt.Sprintf(" — REVIEW: skew %.1f°, fill %.2f, %d part(s)", p.Skew, p.Fill, p.Parts)
		}
		fmt.Println(line)
	}
	if done {
		fmt.Printf("done: %d photo(s) bagged in %s\n", len(ids), filepath.Join(filepath.Dir(filepath.Dir(b.Dir())), "bags"))
		return nil
	}
	if len(prints) == 0 {
		fmt.Println("no prints found on the bed: check", filepath.Join(b.Dir(), "prints.png"))
		return nil
	}
	fmt.Printf("check %s, then:\n  bytestone-scan confirm %s\n", filepath.Join(b.Dir(), "prints.png"), b.ID)
	return nil
}

// telling says what the scanner is doing, since a detail pass takes minutes.
type telling struct{ *epson.Scanner }

func (t *telling) Preview(ctx context.Context) (image.Image, error) {
	fmt.Println("preview: scanning the bed at 75 ppi")
	return t.Scanner.Preview(ctx)
}

func (t *telling) Detail(ctx context.Context, area find.Rect, ppi int) (io.ReadCloser, error) {
	fmt.Printf("detail pass: scanning %.0f × %.0f mm at %d ppi\n", area.X1-area.X0, area.Y1-area.Y0, ppi)
	return t.Scanner.Detail(ctx, area, ppi)
}

// Command bytestone-scan scans batches of prints on the Epson 4990 into
// archival masters, one archived photo per print, run from a web UI.
//
//	bytestone-scan serve [-addr localhost:8080] [-staging DIR] [-archive URL] [-device DEV] [-recorded NAME]
//	bytestone-scan capture -print NAME [-side front|back] [-staging DIR] [-device DEV] [-ppi 2400]
//
// capture records the one print on the bed into <staging>/fixtures/NAME,
// and serve -recorded NAME runs the UI on that recording instead of the
// scanner.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	_ "git.bytestone.uk/hum3/go-postgres"
	"git.bytestone.uk/hum3/gobank-workflow/sqlstore"

	"git.bytestone.uk/hum3/bytestone-scan/internal/archive"
	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
	"git.bytestone.uk/hum3/bytestone-scan/internal/epson"
	"git.bytestone.uk/hum3/bytestone-scan/internal/recorded"
	"git.bytestone.uk/hum3/bytestone-scan/internal/web"
)

const usage = "usage: bytestone-scan serve [flags] | capture -print NAME [flags]   (-h for the flags)"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	home, _ := os.UserHomeDir()
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	staging := fs.String("staging", filepath.Join(home, ".local/share/bytestone-scan"), "staging root: batches, the archive, recordings and the workflow database")
	device := fs.String("device", "", "SANE device; found from scanimage -L if empty")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	var err error
	switch os.Args[1] {
	case "serve":
		addr := fs.String("addr", "localhost:8080", "address to listen on")
		service := fs.String("archive", defaultArchive, "the archive service (life-weaver serve): the register of people, item ids and deposits; left at its default, only the production instance is accepted there")
		name := fs.String("recorded", "", "run on the recording of this print under <staging>/fixtures instead of the scanner")
		fs.Parse(os.Args[2:])
		given := false
		fs.Visit(func(f *flag.Flag) { given = given || f.Name == "archive" })
		var s batch.Scanner = &epson.Scanner{Device: *device}
		if *name != "" {
			s = &recorded.Scanner{Dir: filepath.Join(*staging, "fixtures", *name)}
		}
		err = serve(ctx, *addr, *staging, *service, given, s)
	case "capture":
		name := fs.String("print", "", "the recording's name under <staging>/fixtures (required)")
		side := fs.String("side", "front", "the side of the print lying face down: front or back")
		ppi := fs.Int("ppi", recorded.DetailPPI, "the ppi the print's region is recorded at")
		fs.Parse(os.Args[2:])
		if *name == "" {
			fmt.Fprintln(os.Stderr, usage)
			os.Exit(2)
		}
		err = capture(ctx, filepath.Join(*staging, "fixtures", *name), *side, *device, *ppi)
	default:
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "bytestone-scan:", err)
		os.Exit(1)
	}
}

// serve runs the web UI until interrupted.
// defaultArchive is where the production archive service listens
// (life-weaver serve's own default); a scratch or scale instance is
// served elsewhere, and is only used when -archive names it.
const defaultArchive = "http://localhost:8321"

// acceptArchive is the rule that keeps a scratch register out of real
// batches by accident: at the default URL only production will do, and an
// archive that does not say which instance it is will not; an -archive
// given explicitly accepts any instance.
func acceptArchive(in archive.Instance, url string, given bool) error {
	if given || in.Production() {
		return nil
	}
	return fmt.Errorf("the archive at %s is %s, not production: pass -archive %s to use it anyway", url, in, url)
}

func serve(ctx context.Context, addr, staging, service string, given bool, s batch.Scanner) error {
	client := archive.Client{URL: service}
	if in, err := client.Instance(); err != nil {
		fmt.Fprintln(os.Stderr, "bytestone-scan: cannot tell which archive instance is at", service+":", err)
	} else if err := acceptArchive(in, service, given); err != nil {
		return err
	}
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return err
	}
	db, err := sql.Open("pglike", filepath.Join(staging, "workflow.db"))
	if err != nil {
		return err
	}
	defer db.Close()
	store, err := sqlstore.New(db)
	if err != nil {
		return err
	}
	app := &web.App{
		Staging: staging,
		Store:   store,
		DB:      db,
		Scanner: s,
		Archive: client,
		Now:     time.Now,
	}
	srv := &http.Server{Addr: addr, Handler: app.Handler()}
	go func() {
		<-ctx.Done()
		srv.Close()
	}()
	// closed batches' work files are purged at startup and daily
	go func() {
		for {
			if purged, err := batch.Purge(staging); err != nil {
				fmt.Fprintln(os.Stderr, "bytestone-scan: purge:", err)
			} else if len(purged) > 0 {
				fmt.Println("bytestone-scan: purged the work files of batch", strings.Join(purged, ", "))
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(24 * time.Hour):
			}
		}
	}()
	fmt.Printf("bytestone-scan: http://%s/ (staging %s, %s)\n", addr, staging, s.Name())
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// capture records one side of the print on the bed into dir, keeping the
// exact command lines in commands.txt beside the capture.
func capture(ctx context.Context, dir, side, device string, ppi int) error {
	if err := os.MkdirAll(filepath.Join(dir, side), 0o755); err != nil {
		return err
	}
	trace, err := os.OpenFile(filepath.Join(dir, side, "commands.txt"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer trace.Close()
	fmt.Printf("bytestone-scan: recording the %s into %s at %d ppi; this takes a few minutes\n", side, dir, ppi)
	c, err := recorded.Record(ctx, &epson.Scanner{Device: device, Trace: trace}, dir, side, ppi, time.Now())
	if err != nil {
		return err
	}
	fmt.Printf("print %.0f × %.0f mm at %.0f, %.0f mm, skew %.1f°; recorded %.0f–%.0f × %.0f–%.0f mm\n",
		c.Print.Print.W, c.Print.Print.H, c.Print.Print.Bounds().X0, c.Print.Print.Bounds().Y0, c.Print.Skew,
		c.Bounds.X0, c.Bounds.X1, c.Bounds.Y0, c.Bounds.Y1)
	return nil
}

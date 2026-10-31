// Command bytestone-scan scans batches of prints on the Epson 4990 into
// archival masters, one archived photo per print, run from a web UI.
//
//	bytestone-scan serve [-instance NAME] [-addr HOST:PORT] [-staging DIR] [-archive URL] [-device DEV] [-recorded NAME]
//	bytestone-scan capture -print NAME [-side front|back] [-staging DIR] [-device DEV] [-ppi 2400]
//	bytestone-scan verify [-instance NAME] [-staging DIR] [-archive URL] BATCH[/SCAN]
//
// serve runs for one archive instance, production unless -instance names
// another, found among the life-weaver services on localhost:8321-8330;
// its own port and staging root follow from the instance's slot. capture
// records the one print on the bed into <staging>/fixtures/NAME, and
// serve -recorded NAME runs the UI on that recording instead of the
// scanner. verify asks the archive afresh whether it holds each photo of
// the scan, or of every scan of the batch, as it was receipted.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "git.bytestone.uk/hum3/go-postgres"
	"git.bytestone.uk/hum3/gobank-workflow/sqlstore"

	"git.bytestone.uk/hum3/bytestone-scan/internal/archive"
	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
	"git.bytestone.uk/hum3/bytestone-scan/internal/epson"
	"git.bytestone.uk/hum3/bytestone-scan/internal/recorded"
	"git.bytestone.uk/hum3/bytestone-scan/internal/web"
)

const usage = "usage: bytestone-scan serve [flags] | capture -print NAME [flags] | verify [flags] BATCH[/SCAN]   (-h for the flags)"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	home, _ := os.UserHomeDir()
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	root := filepath.Join(home, ".local/share/bytestone-scan")
	staging := fs.String("staging", "", "staging root: batches, bags, recordings and the workflow database (default "+root+", or "+root+"-<instance> for an instance other than production)")
	device := fs.String("device", "", "SANE device; found from scanimage -L if empty")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	var err error
	switch os.Args[1] {
	case "serve":
		instance := fs.String("instance", "", "the archive instance to scan for, as life-weaver names it (default production), found among those serving on localhost:8321-8330")
		addr := fs.String("addr", "", "address to listen on (default localhost:8080, or 8080+slot for an instance other than production; a port in use is stepped past to the next free one)")
		service := fs.String("archive", "", "the archive service's URL, instead of finding the instance (life-weaver serve): the register of people, item ids and deposits")
		name := fs.String("recorded", "", "run on the recording of this print under <staging>/fixtures instead of the scanner")
		fs.Parse(os.Args[2:])
		var cfg settings
		cfg, err = settings{Instance: *instance, Archive: *service, Addr: *addr, Staging: *staging}.resolve(root, func() []archive.Serving { return archive.Discover(archive.Addresses()) })
		if err != nil {
			break
		}
		var s batch.Scanner = &epson.Scanner{Device: *device}
		if *name != "" {
			s = &recorded.Scanner{Dir: filepath.Join(cfg.Staging, "fixtures", *name)}
		}
		err = serve(ctx, cfg, s)
	case "verify":
		instance := fs.String("instance", "", "the archive instance to verify against, as life-weaver names it (default production), found among those serving on localhost:8321-8330")
		service := fs.String("archive", "", "the archive service's URL, instead of finding the instance")
		fs.Parse(os.Args[2:])
		if fs.NArg() != 1 {
			fmt.Fprintln(os.Stderr, usage)
			os.Exit(2)
		}
		var cfg settings
		cfg, err = settings{Instance: *instance, Archive: *service, Staging: *staging}.resolve(root, func() []archive.Serving { return archive.Discover(archive.Addresses()) })
		if err != nil {
			break
		}
		err = verify(cfg.Staging, fs.Arg(0), archive.Client{URL: cfg.Archive}, os.Stdout)
	case "capture":
		name := fs.String("print", "", "the recording's name under <staging>/fixtures (required)")
		side := fs.String("side", "front", "the side of the print lying face down: front or back")
		ppi := fs.Int("ppi", recorded.DetailPPI, "the ppi the print's region is recorded at")
		fs.Parse(os.Args[2:])
		if *name == "" {
			fmt.Fprintln(os.Stderr, usage)
			os.Exit(2)
		}
		if *staging == "" {
			*staging = root
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
// settings is what serve runs with: the archive instance it is for and,
// each following from the instance's slot unless given, the archive's
// URL, its own address and its staging root.
type settings struct {
	Instance, Archive, Addr, Staging string
}

// resolve fills the settings in from the instance the scanner is run for,
// found among those serving: production unless -instance names another.
// An -archive given explicitly is taken as it is, no instance sought. The
// staging root is root for production and root-<instance> for any other,
// so a scratch batch's masters never land in production's staging.
func (s settings) resolve(root string, discover func() []archive.Serving) (settings, error) {
	slot := 0
	if s.Archive == "" {
		found, err := archive.Choose(s.Instance, discover())
		if err != nil {
			return settings{}, err
		}
		s.Archive, slot = found.URL, found.Instance.Slot
		if !found.Instance.Production() && s.Staging == "" {
			s.Staging = root + "-" + found.Instance.Name
		}
	}
	if s.Addr == "" {
		s.Addr = fmt.Sprintf("localhost:%d", 8080+slot)
	}
	if s.Staging == "" {
		s.Staging = root
	}
	return s, nil
}

func serve(ctx context.Context, cfg settings, s batch.Scanner) error {
	addr, staging, service := cfg.Addr, cfg.Staging, cfg.Archive
	client := archive.Client{URL: service}
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
	l, err := listen(addr, portsTried)
	if err != nil {
		return err
	}
	addr = l.Addr().String()
	srv := &http.Server{Handler: app.Handler()}
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
	if err := srv.Serve(l); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// portsTried bounds listen's search: the instance slots span ten ports.
const portsTried = 10

// listen listens on addr or, when its port is in use, on the next free
// port up, trying at most tries ports; any other failure to listen is
// returned as it is.
func listen(addr string, tries int) (net.Listener, error) {
	host, p, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(p)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", addr, err)
	}
	for i := 0; ; i++ {
		l, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port+i)))
		if err == nil {
			return l, nil
		}
		if !errors.Is(err, syscall.EADDRINUSE) {
			return nil, err
		}
		if i+1 >= tries {
			return nil, fmt.Errorf("listen: ports %d-%d of %s all in use", port, port+i, host)
		}
	}
}

// verify checks the scan named BATCH/SCAN, or every scan of BATCH, against
// the archive, reporting each item on out, and fails when any item is
// not held as it was receipted.
func verify(root, what string, svc archive.Service, out io.Writer) error {
	var scans []*batch.Scan
	if strings.Contains(what, "/") {
		l, err := batch.OpenScan(root, what)
		if err != nil {
			return err
		}
		scans = []*batch.Scan{l}
	} else {
		b, err := batch.Open(root, what)
		if err != nil {
			return err
		}
		if scans, err = b.Scans(); err != nil {
			return err
		}
	}
	if len(scans) == 0 {
		fmt.Fprintf(out, "%s: no scans\n", what)
	}
	failed, items := 0, 0
	for _, l := range scans {
		checks, err := l.Verify(svc)
		if err != nil {
			return err
		}
		failed += report(out, l.Key(), checks)
		items += len(checks)
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d items not held by the archive as receipted", failed, items)
	}
	return nil
}

// report prints one line per check, the scan's key, the item and its
// verdict, and counts the items not held as receipted: those unknown to
// the archive, not held, or holding another checksum. Pending and
// rejected items are said but are not failures.
func report(out io.Writer, key string, checks []batch.Check) int {
	if len(checks) == 0 {
		fmt.Fprintf(out, "%s: nothing to verify\n", key)
		return 0
	}
	failed := 0
	for _, c := range checks {
		what := "photo"
		if c.Page {
			what = "page"
		}
		line := fmt.Sprintf("%s %s %s: %s", key, what, c.Photo, c.Verdict)
		if c.Verdict == batch.Verified {
			line += fmt.Sprintf(", %d files", c.Files)
		}
		if c.Detail != "" {
			line += ", " + c.Detail
		}
		switch c.Verdict {
		case batch.Unknown, batch.NotHeld, batch.Differs:
			failed++
		}
		fmt.Fprintln(out, line)
	}
	return failed
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

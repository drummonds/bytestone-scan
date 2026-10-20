// Command bytestone-scan scans batches of prints on the Epson 4990 into
// archival masters, one bag per photo, run from a web UI.
//
//	bytestone-scan serve [-addr localhost:8080] [-staging DIR] [-people DIR] [-device DEV]
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
	"time"

	_ "git.bytestone.uk/hum3/go-postgres"
	"git.bytestone.uk/hum3/gobank-workflow/sqlstore"

	"git.bytestone.uk/hum3/bytestone-scan/internal/epson"
	"git.bytestone.uk/hum3/bytestone-scan/internal/register"
	"git.bytestone.uk/hum3/bytestone-scan/internal/web"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "serve" {
		fmt.Fprintln(os.Stderr, "usage: bytestone-scan serve [flags]   (-h for the flags)")
		os.Exit(2)
	}
	home, _ := os.UserHomeDir()
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", "localhost:8080", "address to listen on")
	staging := fs.String("staging", filepath.Join(home, ".local/share/bytestone-scan"), "staging root: batches, bags and the workflow database")
	people := fs.String("people", filepath.Join(home, "git/hum3/life-weaver/data/people"), "life-weaver's register of people")
	device := fs.String("device", "", "SANE device; found from scanimage -L if empty")
	fs.Parse(os.Args[2:])

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := serve(ctx, *addr, *staging, *people, *device); err != nil {
		fmt.Fprintln(os.Stderr, "bytestone-scan:", err)
		os.Exit(1)
	}
}

// serve runs the web UI until interrupted.
func serve(ctx context.Context, addr, staging, people, device string) error {
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
		Scanner: &epson.Scanner{Device: device},
		People:  register.Register{Dir: people},
		Now:     time.Now,
	}
	srv := &http.Server{Addr: addr, Handler: app.Handler()}
	go func() {
		<-ctx.Done()
		srv.Close()
	}()
	fmt.Printf("bytestone-scan: http://%s/ (staging %s)\n", addr, staging)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

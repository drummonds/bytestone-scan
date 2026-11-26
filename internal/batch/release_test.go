package batch

import (
	"strings"
	"testing"

	wf "git.bytestone.uk/hum3/gobank-workflow"

	"git.bytestone.uk/hum3/bytestone-scan/internal/archive"
	"git.bytestone.uk/hum3/bytestone-scan/internal/film/filmtest"
	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
	"git.bytestone.uk/hum3/bytestone-scan/internal/writing/writingtest"
)

// notSent is the refusal of a deposit to an archive serving one release
// by a scanner built for another.
func notSent(svc archive.Service, serves, builtFor string) string {
	return "not sent: the archive at " + svc.String() + " serves life-weaver " + serves + ", older than the " + builtFor +
		" this scanner is built for; deploy " + builtFor + " or later to it, then Retry"
}

// Story 211: an item's deposit is not sent to an archive serving a
// life-weaver release older than the one the scanner is built for; the
// item fails at deposit, saying both releases and what to do. The same
// or a newer archive, and one whose release cannot be compared, is
// deposited to as before.
func TestADepositIsNotSentToAnArchiveBehindTheScanner(t *testing.T) {
	for _, c := range []struct {
		serves, builtFor string
		behind           bool
	}{
		{"v0.58.0", "v0.61.0", true},
		{"v0.60.9", "v0.61.0", true},
		{"v0.9.0", "v0.10.0", true},
		{"v0.61.0", "v0.61.0", false},
		{"v0.62.0", "v0.61.0", false},
		{"v1.0.0", "v0.61.0", false},
		{"(devel)", "v0.61.0", false},
		{"", "v0.61.0", false},
		{"v0.58.0", "", false}, // the scanner's build does not say: a test binary
	} {
		t.Run(c.serves+" for "+c.builtFor, func(t *testing.T) {
			root, store := t.TempDir(), wf.NewMemStore()
			svc := service(root)
			svc.Serves = c.serves
			l := describedWriting(t, sitting(t, root), store, "megginch.docx", writingtest.Sample(t))
			_, err := l.Run(t.Context(), Env{Store: store, Archive: svc, Now: clock, BuiltFor: c.builtFor}, nil)
			ids, _ := l.Items()
			if len(ids) != 1 {
				t.Fatalf("items %v, want the writing", ids)
			}
			id := ids[0]
			d, _ := DepositOf(root, id)
			if !c.behind {
				if err != nil || d == nil || d.Status != "held" {
					t.Errorf("deposit %+v, %v, want held", d, err)
				}
				return
			}
			if rec, _ := svc.Receipt(id); err == nil || d != nil || rec.Status != archive.Reserved {
				t.Fatalf("an archive behind: %v, deposit %+v, the archive has it %s: want nothing sent", err, d, rec.Status)
			}
			s, _ := l.Standing(t.Context(), store)
			if want := "writing " + id.String() + " failed at deposit: " + notSent(svc, c.serves, c.builtFor); s.String() != want {
				t.Errorf("standing %q,\nwant %q", s, want)
			}
		})
	}
}

// Story 211: once the release the scanner is built for is deployed to
// the archive, Retry deposits; and an archive that does not answer fails
// the deposit as it did.
func TestADepositRefusedForTheReleaseIsRetried(t *testing.T) {
	root, store := t.TempDir(), wf.NewMemStore()
	svc := service(root)
	svc.Serves = "v0.58.0"
	l := describedWriting(t, sitting(t, root), store, "megginch.docx", writingtest.Sample(t))
	env := Env{Store: store, Archive: svc, Now: clock, BuiltFor: "v0.61.0"}
	if _, err := l.Run(t.Context(), env, nil); err == nil || !strings.Contains(err.Error(), "not sent") {
		t.Fatalf("an archive behind: %v, want not sent", err)
	}
	svc.Down = true
	if _, err := l.Run(t.Context(), env, nil); err == nil || !strings.Contains(err.Error(), "not answering") || strings.Contains(err.Error(), "not sent") {
		t.Errorf("the service down: %v, want not answering", err)
	}
	svc.Down = false
	svc.Serves = "v0.61.0"
	if _, err := l.Run(t.Context(), env, nil); err != nil {
		t.Fatalf("deployed and retried: %v", err)
	}
	if s, _ := l.Standing(t.Context(), store); !s.Through() {
		t.Errorf("standing %q, want through", s)
	}
}

// Story 211: a scan's standing names the item holding it back by its
// kind: a photo, a letter, a film, a writing.
func TestAStandingNamesTheItemByItsKind(t *testing.T) {
	const serves, builtFor = "v0.58.0", "v0.61.0"
	held := func(t *testing.T, kind string, l *Scan, store wf.Store, env Env) {
		t.Helper()
		svc := service(l.root)
		svc.Serves = serves
		env.Store, env.Archive, env.Now, env.BuiltFor = store, svc, clock, builtFor
		if _, err := l.Run(t.Context(), env, nil); err == nil {
			t.Fatal("deposited with an archive behind")
		}
		ids, _ := l.Items()
		s, err := l.Standing(t.Context(), store)
		if want := kind + " " + ids[0].String() + " failed at deposit: " + notSent(svc, serves, builtFor); err != nil || s.String() != want {
			t.Errorf("standing %q, %v,\nwant %q", s, err, want)
		}
	}
	t.Run("a scan of prints", func(t *testing.T) {
		root, store := t.TempDir(), wf.NewMemStore()
		scanner := twoSided()
		b := describe(t, root)
		run(t, b, store, scanner)
		b.Confirm()
		scanned(t, b, store, scanner)
		described(t, b)
		b.Accept()
		held(t, "photo", b, store, Env{Scanner: scanner})
	})
	t.Run("a feed of letters", func(t *testing.T) {
		l, store, f := fedLetter(t, []uint8{40, 255}, sheet)
		if err := l.Accept(); err != nil {
			t.Fatal(err)
		}
		held(t, "letter", l, store, Env{Feeder: f})
	})
	t.Run("a film import", func(t *testing.T) {
		root, store := t.TempDir(), wf.NewMemStore()
		l := importFilm(t, sitting(t, root), filmtest.Plays(t, t.TempDir()))
		runFilm(t, l, store)
		if err := l.ConfirmFilm(t.Context(), FilmDetails{Title: "A Test Film", Transfer: transfer, Poster: Poster{Attached: true}}); err != nil {
			t.Fatal(err)
		}
		runFilm(t, l, store)
		if err := l.DescribeFilm(t.Context(), service(root), FilmDescription{}, now); err != nil {
			t.Fatal(err)
		}
		held(t, "film", l, store, Env{})
	})
	t.Run("a writing import", func(t *testing.T) {
		root, store := t.TempDir(), wf.NewMemStore()
		l := describedWriting(t, sitting(t, root), store, "megginch.docx", writingtest.Sample(t))
		held(t, "writing", l, store, Env{})
	})
}

// Story 211: the kind names the item at every status the standing gives
// one; a scan's own standing names no item.
func TestAStandingNamesTheItemAtEveryStatus(t *testing.T) {
	id := pid.ID(90)
	for _, c := range []struct {
		s    Standing
		want string
	}{
		{Standing{Status: wf.StatusCancelled, Photo: &id, Kind: "writing"}, "writing 2t cancelled"},
		{Standing{Status: wf.StatusWaiting, Step: "describe", Photo: &id, Kind: "writing"}, "writing 2t waiting at describe"},
		{Standing{Status: wf.StatusFailed, Step: "deposit", Photo: &id, Kind: "film"}, "film 2t failed at deposit"},
		{Standing{Status: wf.StatusWaiting, Step: "describe", Kind: "writing"}, "waiting at describe"},
	} {
		if got := c.s.String(); got != c.want {
			t.Errorf("%+v: %q, want %q", c.s, got, c.want)
		}
	}
}

// Story 211: a writing has no renditions, so a deposit of one the
// archive refuses for its renditions, as an archive from before writings
// does, fails with the archive's refusal alone: nothing is remade and
// nothing is said of Retry.
func TestAWritingRefusedForItsRenditionsRemakesNothing(t *testing.T) {
	root, store := t.TempDir(), wf.NewMemStore()
	svc := service(root)
	svc.NoWriting = true
	l := describedWriting(t, sitting(t, root), store, "megginch.docx", writingtest.Sample(t))
	if _, err := l.Run(t.Context(), Env{Store: store, Archive: svc, Now: clock}, nil); err == nil {
		t.Fatal("an archive from before writings held one")
	}
	ids, _ := l.Items()
	s, _ := l.Standing(t.Context(), store)
	want := "writing " + ids[0].String() + " failed at deposit: refused for its renditions: the archive wants renditions/, a rendition missing or of a master not in the bag"
	if s.String() != want {
		t.Errorf("standing %q,\nwant %q", s, want)
	}
	if files(t, l.Renditions(ids[0])) != nil {
		t.Errorf("renditions were made for a writing: %v", files(t, l.Renditions(ids[0])))
	}
}

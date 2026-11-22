package batch

import (
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	wf "git.bytestone.uk/hum3/gobank-workflow"

	"git.bytestone.uk/hum3/bytestone-scan/internal/archive"
	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
)

// Story 191, Verify: a record whose checksum at the archive is not the
// one receipted has changed since the deposit, the archive's doing, and
// the item is verified and says so; a master is never changed since.
// The bag in staging is compared by its masters alone.
func TestVerifyTellsARecordFromAMaster(t *testing.T) {
	root, store := t.TempDir(), wf.NewMemStore()
	_, l := through(t, root, store)
	_, ids, _ := l.Found()
	id := ids[0]
	svc := service(root)
	receipted, _ := svc.Receipt(id)
	held := func(change map[string]string) {
		p := maps.Clone(receipted.Payload)
		for file, sum := range change {
			if sum == "" {
				delete(p, file)
			} else {
				p[file] = sum
			}
		}
		svc.Hold(id, p)
	}
	other := strings.Repeat("0", 64)
	const since = "the record has changed at the archive since the deposit"

	if c := check(t, l, svc, id); c.Verdict != Verified || c.Changed || strings.Contains(c.Detail, since) {
		t.Errorf("every file as receipted: %+v, want verified as before", c)
	}
	held(map[string]string{"data/photo.yaml": other})
	if c := check(t, l, svc, id); c.Verdict != Verified || !c.Changed || !strings.Contains(c.Detail, since) {
		t.Errorf("the record another checksum: %+v, want verified, the record changed since", c)
	}
	svc.NoFixity = true
	if c := check(t, l, svc, id); c.Verdict != Verified || !c.Changed || !strings.Contains(c.Detail, since) || !strings.Contains(c.Detail, "receipt only") {
		t.Errorf("the record another checksum, no fixity check: %+v, want verified, changed since, the receipt only", c)
	}
	svc.NoFixity = false
	svc.Rotten[id] = []string{"data/photo.yaml"}
	if c := check(t, l, svc, id); c.Verdict != Differs || !strings.Contains(c.Detail, "at the archive, data/photo.yaml") {
		t.Errorf("the record another checksum, the archive's fixity not sound: %+v, want differs with its failures", c)
	}
	delete(svc.Rotten, id)

	held(map[string]string{"data/photo.yaml": other, "data/front.tif": other})
	if c := check(t, l, svc, id); c.Verdict != Differs || !strings.Contains(c.Detail, "data/front.tif") || !strings.Contains(c.Detail, "not the one receipted") {
		t.Errorf("a master another checksum: %+v, want differs naming the master", c)
	}
	held(map[string]string{"data/photo.yaml": ""})
	if c := check(t, l, svc, id); c.Verdict != Differs || !strings.Contains(c.Detail, "data/photo.yaml") {
		t.Errorf("a file receipted is not held: %+v, want differs naming it", c)
	}
	held(map[string]string{"data/extra.yaml": other})
	if c := check(t, l, svc, id); c.Verdict != Differs || !strings.Contains(c.Detail, "data/extra.yaml") {
		t.Errorf("a file held that was not receipted: %+v, want differs naming it", c)
	}

	held(map[string]string{"data/photo.yaml": other})
	os.RemoveAll(filepath.Join(root, "bags", id.String())) // purged
	if c := check(t, l, svc, id); c.Verdict != Verified || !c.Changed || !strings.Contains(c.Detail, since) {
		t.Errorf("the record changed since, the bag purged: %+v, want verified, changed since", c)
	}
}

// Story 191, Home now: the home Verify gives is the one the archive
// gives now, beside the home as sent; nothing local is rewritten.
func TestVerifyGivesTheHomeNow(t *testing.T) {
	root, store := t.TempDir(), wf.NewMemStore()
	b, l := through(t, root, store)
	_, ids, _ := l.Found()
	id := ids[0]
	svc := service(root)
	local := []string{filepath.Join(b.Dir(), "batch.yaml"), filepath.Join(root, "bags", id.String(), "data", "photo.yaml"), filepath.Join(root, "bags", id.String()+".deposit.yaml")}
	var before []string
	for _, f := range local {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		before = append(before, string(raw))
	}
	if c := check(t, l, svc, id); c.Home != 10 || c.Sent != 10 {
		t.Errorf("the archive answering the home as sent: %+v, want home and sent 10", c)
	}
	svc.Renumber(10, 11)
	if c := check(t, l, svc, id); c.Verdict != Verified || !c.Changed || c.Home != 11 || c.Sent != 10 {
		t.Errorf("after the renumber: %+v, want verified, the record changed, home 11, sent under 10", c)
	}
	for i, f := range local {
		if raw, _ := os.ReadFile(f); string(raw) != before[i] {
			t.Errorf("%s was rewritten", f)
		}
	}
}

// packagedUnder is a batch homed with Ada (10) holding one scan through,
// its photos deposited, and a second whose photos are packaged and not
// deposited: the service was down.
func packagedUnder(t *testing.T) (root string, store wf.Store, b *Batch, deposited, waiting *Scan) {
	t.Helper()
	root, store = t.TempDir(), wf.NewMemStore()
	b, deposited = through(t, root, store)
	scanner, svc := twoSided(), service(root)
	waiting, err := b.NewScan("", now)
	if err != nil {
		t.Fatal(err)
	}
	run(t, waiting, store, scanner)
	waiting.Confirm()
	scanned(t, waiting, store, scanner)
	described(t, waiting)
	waiting.Accept()
	svc.Down = true
	if _, err := waiting.Run(context.Background(), Env{Store: store, Scanner: scanner, Archive: svc, Now: clock}, nil); err == nil {
		t.Fatal("deposited to a service that is down")
	}
	svc.Down = false
	return root, store, b, deposited, waiting
}

// Story 191: an open batch whose person the register no longer holds
// begins nothing new and deposits nothing until it is re-homed; what is
// packaged and not deposited is packaged again with the home chosen, and
// what is deposited is left as sent.
func TestAnOpenBatchWhosePersonIsGoneIsRehomed(t *testing.T) {
	root, store, b, deposited, waiting := packagedUnder(t)
	svc, scanner := service(root), twoSided()
	_, sent, _ := deposited.Found()
	_, ids, _ := waiting.Found()

	if err := b.Homed(svc); err != nil {
		t.Errorf("a batch whose person the register holds: %v", err)
	}
	if err := b.Rehome(svc, 14); err == nil {
		t.Error("re-homed a batch whose person the register holds")
	}

	svc.Renumber(10, 11)
	err := b.Homed(svc)
	if !errors.Is(err, ErrRehome) || !strings.Contains(err.Error(), "person a is not in the register") {
		t.Errorf("a batch whose person is gone: %v, want refused, person a not in the register, Re-home", err)
	}
	// a deposit is not sent for a home the archive does not give the id
	_, err = waiting.Run(context.Background(), Env{Store: store, Scanner: scanner, Archive: svc, Now: clock}, nil)
	if err == nil || !strings.Contains(err.Error(), "the batch names a, the archive b") {
		t.Errorf("a deposit under a home the archive does not give the id: %v, want refused naming both", err)
	}
	for _, id := range ids {
		if rec, _ := svc.Receipt(id); rec.Status != archive.Reserved {
			t.Errorf("item %s was sent: %+v", id, rec)
		}
		if d, _ := DepositOf(root, id); d != nil {
			t.Errorf("item %s has an answer: %+v", id, d)
		}
	}

	kept := map[string]string{}
	for _, f := range []string{filepath.Join("bags", sent[0].String(), "data", "photo.yaml"), filepath.Join("bags", sent[0].String(), "manifest-sha256.txt"), filepath.Join("bags", sent[0].String()+".deposit.yaml")} {
		raw, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			t.Fatal(err)
		}
		kept[f] = string(raw)
	}
	for _, person := range []pid.ID{0, 12} {
		if err := b.Rehome(svc, person); err == nil {
			t.Errorf("re-homed with %s, who is no one", person)
		}
	}
	if again, _ := Open(root, b.ID); again.Person != 10 {
		t.Errorf("a refused re-home changed the batch: person %s", again.Person)
	}
	if err := b.Rehome(svc, 11); err != nil {
		t.Fatal(err)
	}
	again, err := Open(root, b.ID)
	if err != nil || again.Person != 11 || again.PersonName != "Ada Example" {
		t.Errorf("batch.yaml after the re-home: %+v %v, want person b, Ada Example", again, err)
	}
	if err := again.Homed(svc); err != nil {
		t.Errorf("a re-homed batch: %v", err)
	}
	for f, was := range kept {
		if raw, _ := os.ReadFile(filepath.Join(root, f)); string(raw) != was {
			t.Errorf("%s, of a photo deposited, was rewritten", f)
		}
	}
	if p := readPhoto(t, filepath.Join(root, "bags", ids[0].String(), "data", "photo.yaml")); p.ID != ids[0] || p.Home.ID != 11 || p.Home.Name != "Ada Example" {
		t.Errorf("photo %s packaged again: id %s home %+v, want home b", ids[0], p.ID, p.Home)
	}
	if waiting, err = again.Scan(waiting.N); err != nil {
		t.Fatal(err)
	}
	run(t, waiting, store, scanner)
	for _, id := range ids {
		if d, _ := DepositOf(root, id); d == nil || d.Status != "held" || d.Home != 11 {
			t.Errorf("item %s after the re-home: %+v, want held, home b", id, d)
		}
	}
}

// Story 191: a photo scanned and not yet packaged is re-homed too, so
// that it is not packaged under the person who is gone.
func TestRehomeReachesAPhotoNotYetPackaged(t *testing.T) {
	root, store := t.TempDir(), wf.NewMemStore()
	scanner, svc := twoSided(), service(root)
	l := describe(t, root)
	run(t, l, store, scanner)
	l.Confirm()
	scanned(t, l, store, scanner)
	svc.Renumber(10, 11)
	if err := l.Sitting().Rehome(svc, 11); err != nil {
		t.Fatal(err)
	}
	b, _ := Open(root, l.Batch)
	l, _ = b.Scan(l.N)
	described(t, l)
	l.Accept()
	if rec := run(t, l, store, scanner); rec.Status != wf.StatusCompleted {
		t.Fatalf("scan: %s %s", rec.Status, rec.Error)
	}
	_, ids, _ := l.Found()
	for _, id := range ids {
		if p := readPhoto(t, filepath.Join(root, "bags", id.String(), "data", "photo.yaml")); p.Home.ID != 11 {
			t.Errorf("photo %s: home %s, want b", id, p.Home.ID)
		}
	}
}

// Story 191: a finished batch is not re-homed: nothing more will be
// deposited from it.
func TestAFinishedBatchIsNotRehomed(t *testing.T) {
	root, store := t.TempDir(), wf.NewMemStore()
	b, _ := through(t, root, store)
	svc := service(root)
	if _, err := b.Finish(context.Background(), store); err != nil {
		t.Fatal(err)
	}
	svc.Renumber(10, 11)
	if err := b.Rehome(svc, 11); err == nil || !strings.Contains(err.Error(), "finished") {
		t.Errorf("re-homed a finished batch: %v", err)
	}
}

// Story 191: a deposit for an id the archive does not know is refused,
// nothing sent.
func TestADepositForAnIdTheArchiveDoesNotKnowIsRefused(t *testing.T) {
	root, store, _, _, waiting := packagedUnder(t)
	svc := service(root)
	_, ids, _ := waiting.Found()
	svc.Forget(ids[0])
	err := waiting.deposit(ids[0], Env{Store: store, Scanner: twoSided(), Archive: svc, Now: clock})
	if !errors.Is(err, archive.ErrUnknown) {
		t.Errorf("a deposit for an id the archive does not know: %v, want refused as unknown", err)
	}
	if d, _ := DepositOf(root, ids[0]); d != nil {
		t.Errorf("an answer was kept: %+v", d)
	}
}

// Story 191, No id: 0 is never chosen or written as a person's id, nor
// as the item a photo is a duplicate of; read from an older file it
// means none was given, or the id is gone.
func TestZeroIsNoID(t *testing.T) {
	root, store := t.TempDir(), wf.NewMemStore()
	svc := service(root)
	svc.Register = append(svc.Register, archive.Person{ID: 0, Name: "Nobody"})
	_, err := Describe(root, svc, nil, Description{Text: "a shoebox", Person: 0, ScanPPI: 600, Credit: "Drummond family collection"}, now)
	if err == nil || !strings.Contains(err.Error(), "0 is no id") {
		t.Errorf("a batch homed with 0: %v, want refused, 0 is no id", err)
	}

	b, l := through(t, root, store, 7)
	_, ids, _ := l.Found()
	dup, held := ids[0], ids[1]
	raw, _ := os.ReadFile(filepath.Join(root, "bags", held.String()+".deposit.yaml"))
	if strings.Contains(string(raw), "duplicate_of") {
		t.Errorf("the answer for an item held names a duplicate:\n%s", raw)
	}

	// the stores as they were: batch.yaml and the answers name 0
	yaml := filepath.Join(b.Dir(), "batch.yaml")
	raw, _ = os.ReadFile(yaml)
	os.WriteFile(yaml, []byte(strings.Replace(string(raw), "person: a\n", `person: "0"`+"\n", 1)), 0o644)
	old, err := Open(root, b.ID)
	if err != nil || old.Person != 0 {
		t.Fatalf("a batch.yaml naming person 0: %+v %v, want it to load", old, err)
	}
	if err := old.Homed(svc); !errors.Is(err, ErrRehome) {
		t.Errorf("a batch naming 0, the register holding a 0: %v, want Re-home: 0 is no id", err)
	}
	answer := filepath.Join(root, "bags", held.String()+".deposit.yaml")
	raw, _ = os.ReadFile(answer)
	os.WriteFile(answer, append(raw, []byte("duplicate_of: \"0\"\n")...), 0o644)
	if c := check(t, l, svc, held); c.Verdict != Verified {
		t.Errorf("an answer held, duplicate_of 0: %+v, want verified: no duplicate", c)
	}
	answer = filepath.Join(root, "bags", dup.String()+".deposit.yaml")
	raw, _ = os.ReadFile(answer)
	os.WriteFile(answer, []byte(strings.Replace(string(raw), "duplicate_of: \"7\"", "duplicate_of: \"0\"", 1)), 0o644)
	if d, _ := DepositOf(root, dup); d == nil || d.DuplicateOf != 0 {
		t.Fatalf("the answer was not rewritten for the test: %+v\n%s", d, raw)
	}
	if c := check(t, l, svc, dup); c.Verdict != Rejected || strings.Contains(c.Detail, "item 0") || !strings.Contains(c.Detail, "gone") {
		t.Errorf("an answer duplicate, duplicate_of 0: %+v, want rejected, naming no item", c)
	}
}

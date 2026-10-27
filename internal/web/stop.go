package web

import (
	"context"
	"errors"
	"image"
	"io"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
	"git.bytestone.uk/hum3/bytestone-scan/internal/find"
)

// errStopped is what a pass ends with when the operator stops the step
// running: the run records it, and the scan page reads it as a stop, not
// a failure.
var errStopped = errors.New("stopped by the operator")

// stoppable is the scanner as a running step sees it: a pass ends with
// errStopped once stop is closed, the scanner's own context cancelled so
// scanimage is killed. The workflow's own context is left alone, so the
// stop is recorded.
type stoppable struct {
	batch.Scanner
	stop <-chan struct{}
}

func (s stoppable) stopped() bool {
	select {
	case <-s.stop:
		return true
	default:
		return false
	}
}

// until is ctx, cancelled when the step is stopped.
func (s stoppable) until(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		select {
		case <-s.stop:
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
}

func (s stoppable) Preview(ctx context.Context) (image.Image, error) {
	ctx, cancel := s.until(ctx)
	defer cancel()
	img, err := s.Scanner.Preview(ctx)
	if err != nil && s.stopped() {
		return nil, errStopped
	}
	return img, err
}

func (s stoppable) Detail(ctx context.Context, area find.Rect, ppi int) (io.ReadCloser, error) {
	ctx, cancel := s.until(ctx)
	rc, err := s.Scanner.Detail(ctx, area, ppi)
	if err != nil {
		cancel()
		if s.stopped() {
			return nil, errStopped
		}
		return nil, err
	}
	return &stoppedStream{rc, cancel, s}, nil
}

// TurnTo turns the fake scanner's prints, for the tests.
func (s stoppable) TurnTo(side string) {
	if t, ok := s.Scanner.(batch.Turnable); ok {
		t.TurnTo(side)
	}
}

// stoppedStream is a detail pass's stream whose reads and close report
// the stop rather than the broken pipe it leaves.
type stoppedStream struct {
	io.ReadCloser
	cancel context.CancelFunc
	s      stoppable
}

func (st *stoppedStream) Read(p []byte) (int, error) {
	n, err := st.ReadCloser.Read(p)
	if err != nil && st.s.stopped() {
		return n, errStopped
	}
	return n, err
}

func (st *stoppedStream) Close() error {
	err := st.ReadCloser.Close()
	st.cancel()
	if st.s.stopped() {
		return errStopped
	}
	return err
}

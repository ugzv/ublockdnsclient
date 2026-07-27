package runtime

import (
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// stallingBody blocks in Read until it is closed, standing in for an SSE
// connection whose server stopped sending keepalives without dropping the TCP
// connection.
type stallingBody struct {
	unblock   chan struct{}
	closeOnce sync.Once
}

func newStallingBody() *stallingBody {
	return &stallingBody{unblock: make(chan struct{})}
}

func (b *stallingBody) Read([]byte) (int, error) {
	<-b.unblock
	return 0, errors.New("use of closed network connection")
}

func (b *stallingBody) Close() error {
	b.closeOnce.Do(func() { close(b.unblock) })
	return nil
}

func TestIdleReaderFailsStalledStream(t *testing.T) {
	r := newIdleReader(newStallingBody(), 20*time.Millisecond)
	t.Cleanup(func() { _ = r.Close() })

	_, err := r.Read(make([]byte, 16))
	if err == nil {
		t.Fatal("Read() returned no error for a stalled stream")
	}
	if !strings.Contains(err.Error(), "idle") {
		t.Fatalf("Read() error = %v, want it to name the idle timeout", err)
	}
}

// chunkedBody returns one chunk per Read, pausing between them, so the total
// stream outlives the idle timeout while no single gap does.
type chunkedBody struct {
	chunks [][]byte
	pause  time.Duration
	i      int
}

func (b *chunkedBody) Read(p []byte) (int, error) {
	if b.i >= len(b.chunks) {
		return 0, io.EOF
	}
	time.Sleep(b.pause)
	n := copy(p, b.chunks[b.i])
	b.i++
	return n, nil
}

func (b *chunkedBody) Close() error { return nil }

func TestIdleReaderResetsDeadlineOnData(t *testing.T) {
	body := &chunkedBody{
		chunks: [][]byte{[]byte("a"), []byte("b"), []byte("c"), []byte("d")},
		pause:  15 * time.Millisecond,
	}
	r := newIdleReader(body, 60*time.Millisecond)
	t.Cleanup(func() { _ = r.Close() })

	// Total duration exceeds the timeout; each individual gap does not.
	for range body.chunks {
		if _, err := r.Read(make([]byte, 16)); err != nil {
			t.Fatalf("Read() error = %v, want the deadline reset by incoming data", err)
		}
	}
	if _, err := r.Read(make([]byte, 16)); !errors.Is(err, io.EOF) {
		t.Fatalf("Read() error = %v, want io.EOF", err)
	}
}

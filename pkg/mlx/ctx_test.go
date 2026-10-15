package mlx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iksnerd/ava/internal/protocol"
)

// hangingServer accepts a request and never answers it until the test ends,
// like an engine mid-synthesis. entered fires when a request arrives.
func hangingServer(t *testing.T) (*Client, <-chan struct{}) {
	t.Helper()
	entered := make(chan struct{}, 4)
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { close(release); srv.Close() })
	return NewClient(srv.URL), entered
}

// returnsPromptly runs fn, cancels once the server has the request, and fails
// if fn is still running a second later (the client's own timeout is 300s, so
// only the context can end it).
func returnsPromptly(t *testing.T, entered <-chan struct{}, cancel context.CancelFunc, fn func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("the request never reached the server")
	}
	cancel()
	select {
	case err := <-done:
		return err
	case <-time.After(time.Second):
		t.Fatal("the call did not return after its context was cancelled")
		return nil
	}
}

func TestSpeakAbortsWhenItsContextIsCancelled(t *testing.T) {
	c, entered := hangingServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := returnsPromptly(t, entered, cancel, func() error {
		_, err := c.Speak(ctx, SpeakOptions{Text: "x"})
		return err
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Speak() error = %v, want it to wrap context.Canceled", err)
	}
}

func TestSpeakTimedAbortsWhenItsContextIsCancelled(t *testing.T) {
	c, entered := hangingServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := returnsPromptly(t, entered, cancel, func() error {
		_, err := c.SpeakTimed(ctx, SpeakOptions{Text: "x"})
		return err
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("SpeakTimed() error = %v, want it to wrap context.Canceled", err)
	}
}

func TestCheckAbortsWhenItsContextIsCancelled(t *testing.T) {
	c, entered := hangingServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan State, 1)
	go func() { done <- c.Check(ctx) }()
	<-entered
	cancel()
	select {
	case got := <-done:
		if got != Down {
			t.Errorf("Check() on a cancelled context = %v, want Down", got)
		}
	case <-time.After(time.Second):
		// healthTimeout is 2s, so a Check that ignores ctx is still running.
		t.Fatal("Check did not return after its context was cancelled")
	}
}

// A context that is already over must not even reach the wire. The server here
// answers at once, so a client that ignores the context fails the test instead
// of hanging it.
func TestACancelledContextSendsNoRequest(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(`{"protocol":"` + protocol.EngineProtocol + `"}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := c.Speak(ctx, SpeakOptions{Text: "x"}); !errors.Is(err, context.Canceled) {
		t.Errorf("Speak() error = %v, want context.Canceled", err)
	}
	if _, err := c.SpeakTimed(ctx, SpeakOptions{Text: "x"}); !errors.Is(err, context.Canceled) {
		t.Errorf("SpeakTimed() error = %v, want context.Canceled", err)
	}
	if c.Healthy(ctx) {
		t.Error("Healthy() = true on a cancelled context")
	}
	if n := requests.Load(); n != 0 {
		t.Errorf("%d requests reached the server on a cancelled context", n)
	}
}

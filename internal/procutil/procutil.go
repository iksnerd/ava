// Package procutil holds small subprocess helpers shared across the CLIs
// and engine clients.
package procutil

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// Silence redirects cmd's stdout and stderr to /dev/null. Call the returned
// closer once cmd has finished running to release the descriptor.
func Silence(cmd *exec.Cmd) (func(), error) {
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		return nil, err
	}
	cmd.Stdout = devNull
	cmd.Stderr = devNull
	return func() { devNull.Close() }, nil
}

// OnInterrupt spawns a goroutine that waits for SIGINT or SIGTERM and then
// calls onSignal. It returns immediately; typical use is to register cleanup
// before starting a blocking subprocess or server that the same signal will
// also terminate.
func OnInterrupt(onSignal func()) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		onSignal()
	}()
}

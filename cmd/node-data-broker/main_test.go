package main

import (
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/topograph/pkg/test"
)

func TestMain(t *testing.T) {
	port, err := test.GetAvailablePort()
	require.NoError(t, err)

	ch := make(chan error)
	go func() {
		ch <- mainInternal(port)
	}()
	time.Sleep(time.Second)

	err = syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	require.NoError(t, err)

	err = <-ch
	require.EqualError(t, err, "received signal interrupt")
}

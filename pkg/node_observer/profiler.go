package node_observer

import (
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"

	"k8s.io/klog/v2"
)

type Profiler struct {
	listener net.Listener
}

func NewProfilingServer(port int) *Profiler {
	// Listen on the specified port for pprof profiling
	addr := net.JoinHostPort("localhost", fmt.Sprintf("%d", port))
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		klog.Fatalf("Failed to start profiling server on %s: %v", addr, err)
	}

	return &Profiler{
		listener: listener,
	}
}

func (c *Profiler) Start() error {
	// Start the pprof server
	err := http.Serve(c.listener, nil) // DefaultServeMux will handle pprof endpoints
	if err != nil {
		klog.Errorf("Failed to start pprof server: %v", err)
		return err
	}
	klog.Infof("Pprofiler server started on %s", c.listener.Addr().String())
	return nil
}

func (c *Profiler) Stop(err error) {
	klog.Infof("Stopping Pprofiler server: %v", err)
	c.listener.Close()
}

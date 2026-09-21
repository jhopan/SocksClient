package main

import (
	"net"
	"testing"
	"time"
)

// The watchdog exists for one failure mode: the machine changes networks while
// the tunnel keeps running against a dead route. Feed it a probe that reports a
// different interface index and it must fire exactly once.
func TestWatchNetworkFiresOnInterfaceChange(t *testing.T) {
	var (
		stop     = make(chan struct{})
		fired    = make(chan string, 4)
		indexVal = uint32(7)
	)
	probe := func(net.IP) (uint32, error) { return indexVal, nil }

	go watchNetwork("10.12.132.225", 10*time.Millisecond, probe, stop,
		func(reason string) { fired <- reason })

	time.Sleep(40 * time.Millisecond) // stable: nothing may fire
	select {
	case r := <-fired:
		t.Fatalf("watchdog fired without a change: %s", r)
	default:
	}

	indexVal = 11
	select {
	case <-fired:
	case <-time.After(2 * time.Second):
		t.Fatal("watchdog did not fire after the interface changed")
	}
	close(stop)
}

// A hostname has no interface index to compare: the watchdog must stay quiet
// instead of guessing and restarting the tunnel for nothing.
func TestWatchNetworkIgnoresHostnames(t *testing.T) {
	fired := make(chan string, 1)
	stop := make(chan struct{})
	defer close(stop)
	go watchNetwork("vpn.example.com", 5*time.Millisecond, func(net.IP) (uint32, error) {
		return 1, nil
	}, stop, func(reason string) { fired <- reason })

	select {
	case r := <-fired:
		t.Fatalf("watchdog fired for a hostname: %s", r)
	case <-time.After(60 * time.Millisecond):
	}
}

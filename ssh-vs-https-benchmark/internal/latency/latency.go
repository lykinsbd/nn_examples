// Package latency provides a net.Listener wrapper that adds artificial
// one-way delay to every connection, simulating real network latency.
//
// The delay is applied to both Read and Write, so a configured delay of
// 35ms produces ~70ms RTT — matching real-world measurements.
package latency

import (
	"net"
	"time"
)

// Listener wraps a net.Listener, injecting one-way delay on accepted connections.
type Listener struct {
	net.Listener
	Delay time.Duration
}

// Accept waits for and returns the next connection, wrapped with delay.
func (l *Listener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	if l.Delay <= 0 {
		return c, nil
	}
	return &delayConn{Conn: c, delay: l.Delay}, nil
}

// delayConn wraps a net.Conn, adding one-way delay to Read and Write.
type delayConn struct {
	net.Conn
	delay time.Duration
}

func (c *delayConn) Read(b []byte) (int, error) {
	time.Sleep(c.delay)
	return c.Conn.Read(b)
}

func (c *delayConn) Write(b []byte) (int, error) {
	time.Sleep(c.delay)
	return c.Conn.Write(b)
}

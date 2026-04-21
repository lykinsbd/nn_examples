// Package proxy provides an HTTPS server that forwards CLI commands
// to a backend network device over SSH. This is the "edge proxy" pattern:
// automation talks HTTPS over the WAN, the proxy talks SSH over a
// low-latency campus link.
package proxy

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// Server is an HTTPS proxy that forwards commands to a backend SSH device.
type Server struct {
	addr       string
	backendAddr string
	sshCfg     *ssh.ClientConfig
	pooled     bool
	mu         sync.Mutex
	pool       *ssh.Client
	listener   net.Listener
}

// New creates a proxy server. If pooled is true, one SSH connection
// is reused across requests; otherwise each request gets a fresh one.
func New(addr, backendAddr, user, pass string, pooled bool) *Server {
	return &Server{
		addr:        addr,
		backendAddr: backendAddr,
		pooled:      pooled,
		sshCfg: &ssh.ClientConfig{
			User:            user,
			Auth:            []ssh.AuthMethod{ssh.Password(pass)},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         10 * time.Second,
		},
	}
}

// SetListener sets a custom net.Listener (e.g., with latency injection).
func (s *Server) SetListener(ln net.Listener) { s.listener = ln }

// ListenAndServeTLS starts the HTTPS proxy.
func (s *Server) ListenAndServeTLS() error {
	tlsCfg, err := selfSignedTLS()
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/admin/exec/", s.handleExec)
	mux.HandleFunc("/admin/config", s.handleConfig)

	baseLn := s.listener
	if baseLn == nil {
		baseLn, err = net.Listen("tcp", s.addr)
		if err != nil {
			return err
		}
	}
	ln := tls.NewListener(baseLn, tlsCfg)
	log.Printf("Proxy HTTPS listening on %s → SSH backend %s (pooled=%v)", baseLn.Addr(), s.backendAddr, s.pooled)
	return (&http.Server{Handler: mux, TLSConfig: tlsCfg}).Serve(ln)
}

func (s *Server) getSSH() (*ssh.Client, bool, error) {
	if !s.pooled {
		c, err := ssh.Dial("tcp", s.backendAddr, s.sshCfg)
		return c, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pool != nil {
		return s.pool, true, nil
	}
	c, err := ssh.Dial("tcp", s.backendAddr, s.sshCfg)
	if err != nil {
		return nil, false, err
	}
	s.pool = c
	return c, true, nil
}

func (s *Server) execSSH(commands []string) (string, error) {
	conn, pooled, err := s.getSSH()
	if err != nil {
		return "", fmt.Errorf("ssh dial: %w", err)
	}
	if !pooled {
		defer conn.Close()
	}

	var out strings.Builder
	for _, cmd := range commands {
		sess, err := conn.NewSession()
		if err != nil {
			return out.String(), fmt.Errorf("ssh session: %w", err)
		}
		b, err := sess.Output(cmd)
		sess.Close()
		if err != nil {
			return out.String(), fmt.Errorf("ssh exec: %w", err)
		}
		out.Write(b)
	}
	return out.String(), nil
}

func (s *Server) handleExec(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/admin/exec/")
	if path == "" {
		http.Error(w, "no command", http.StatusBadRequest)
		return
	}
	var cmds []string
	for _, p := range strings.Split(path, "/") {
		cmd := strings.TrimSpace(strings.ReplaceAll(p, "+", " "))
		if cmd != "" {
			cmds = append(cmds, cmd)
		}
	}
	out, err := s.execSSH(cmds)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	io.WriteString(w, out)
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var cmds []string
	for _, line := range strings.Split(string(body), "\n") {
		cmd := strings.TrimSpace(line)
		if cmd != "" {
			cmds = append(cmds, cmd)
		}
	}
	out, err := s.execSSH(cmds)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	io.WriteString(w, out)
}

func selfSignedTLS() (*tls.Config, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
	}, nil
}

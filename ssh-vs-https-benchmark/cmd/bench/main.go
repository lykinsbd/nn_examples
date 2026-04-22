package main

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lykinsbd/nn_examples/ssh-vs-https-benchmark/internal/device"
	"github.com/lykinsbd/nn_examples/ssh-vs-https-benchmark/internal/httpserver"
	latencyPkg "github.com/lykinsbd/nn_examples/ssh-vs-https-benchmark/internal/latency"
	"github.com/lykinsbd/nn_examples/ssh-vs-https-benchmark/internal/proxy"
	"github.com/lykinsbd/nn_examples/ssh-vs-https-benchmark/internal/sshserver"
	"golang.org/x/crypto/ssh"
)

// errDuration is a sentinel value indicating a failed iteration.
const errDuration = time.Duration(-1)

type Result struct {
	Transport   string  `json:"transport"`
	Operation   string  `json:"operation"`
	Commands    int     `json:"commands"`
	Iterations  int     `json:"iterations"`
	Errors      int     `json:"errors"`
	Concurrency int     `json:"concurrency"`
	Latency     string  `json:"latency_profile"`
	RTTms       float64 `json:"simulated_rtt_ms"`
	AvgMs       float64 `json:"avg_ms"`
	MinMs       float64 `json:"min_ms"`
	MaxMs       float64 `json:"max_ms"`
	P50Ms       float64 `json:"p50_ms"`
	P95Ms       float64 `json:"p95_ms"`
	StddevMs    float64 `json:"stddev_ms"`
}

// Latency profiles sourced from Verizon Enterprise monthly backbone
// measurements (March 2026) and AWS/RIPE Atlas data.
// See plans/ssh-vs-https-cli/latency-profiles.md for full citations.
var latencyProfiles = map[string]time.Duration{
	"local":            0,
	"campus":           1 * time.Millisecond,
	"regional":         15 * time.Millisecond,
	"continental":      35 * time.Millisecond,
	"intercontinental": 75 * time.Millisecond,
	"transpacific":     87 * time.Millisecond,
}

func main() {
	sshPort := flag.Int("ssh-port", 2222, "SSH listen port (embedded mode)")
	httpsPort := flag.Int("https-port", 8443, "HTTPS listen port (embedded mode)")
	user := flag.String("user", "admin", "Username")
	pass := flag.String("pass", "admin", "Password")
	transport := flag.String("transport", "both", "Transport: ssh, https, both, proxy")
	iterations := flag.Int("iterations", 50, "Iterations per test")
	concurrency := flag.Int("concurrency", 1, "Concurrent workers")
	commands := flag.Int("commands", 1, "Commands per iteration")
	profile := flag.String("latency", "local", "Latency profile")
	proxyPort := flag.Int("proxy-port", 9443, "Proxy HTTPS listen port")
	transcriptsDir := flag.String("transcripts", "transcripts", "Transcript dir")
	flag.Parse()

	delay, ok := latencyProfiles[*profile]
	if !ok {
		log.Fatalf("unknown latency profile %q", *profile)
	}
	rttMs := float64(delay.Milliseconds()) * 2

	sshAddr := fmt.Sprintf("localhost:%d", *sshPort)
	httpsAddr := fmt.Sprintf("localhost:%d", *httpsPort)

	dev, err := device.New("bench-rtr", *user, *pass, *transcriptsDir)
	if err != nil {
		log.Fatalf("device: %v", err)
	}

	// Start SSH server
	sshLn, err := net.Listen("tcp", sshAddr)
	if err != nil {
		log.Fatalf("ssh listen: %v", err)
	}
	sshSrv, err := sshserver.New(sshAddr, dev)
	if err != nil {
		log.Fatalf("ssh: %v", err)
	}
	sshSrv.SetListener(&latencyPkg.Listener{Listener: sshLn, Delay: delay})
	go sshSrv.ListenAndServe()

	// Start HTTPS server
	httpsLn, err := net.Listen("tcp", httpsAddr)
	if err != nil {
		log.Fatalf("https listen: %v", err)
	}
	httpSrv := httpserver.New(httpsAddr, dev)
	httpSrv.SetListener(&latencyPkg.Listener{Listener: httpsLn, Delay: delay})
	go httpSrv.ListenAndServeTLS()

	// Proxy: HTTPS frontend (WAN latency) → SSH backend (campus latency)
	proxyAddr := fmt.Sprintf("localhost:%d", *proxyPort)
	backendSSHPort := *sshPort + 1000
	backendSSHAddr := fmt.Sprintf("localhost:%d", backendSSHPort)
	backendLn, err := net.Listen("tcp", backendSSHAddr)
	if err != nil {
		log.Fatalf("backend ssh listen: %v", err)
	}
	backendSrv, err := sshserver.New(backendSSHAddr, dev)
	if err != nil {
		log.Fatalf("backend ssh: %v", err)
	}
	backendSrv.SetListener(&latencyPkg.Listener{Listener: backendLn, Delay: 1 * time.Millisecond})
	go backendSrv.ListenAndServe()

	proxyLn, err := net.Listen("tcp", proxyAddr)
	if err != nil {
		log.Fatalf("proxy listen: %v", err)
	}
	proxyFresh := proxy.New(proxyAddr, backendSSHAddr, *user, *pass, false)
	proxyFresh.SetListener(&latencyPkg.Listener{Listener: proxyLn, Delay: delay})
	go proxyFresh.ListenAndServeTLS()

	proxyPooledAddr := fmt.Sprintf("localhost:%d", *proxyPort+1)
	proxyPooledLn, err := net.Listen("tcp", proxyPooledAddr)
	if err != nil {
		log.Fatalf("proxy-pooled listen: %v", err)
	}
	proxyPooled := proxy.New(proxyPooledAddr, backendSSHAddr, *user, *pass, true)
	proxyPooled.SetListener(&latencyPkg.Listener{Listener: proxyPooledLn, Delay: delay})
	go proxyPooled.ListenAndServeTLS()

	time.Sleep(500 * time.Millisecond)
	log.Printf("Server ready — profile=%s, simulated RTT=%.0fms", *profile, rttMs)

	var results []Result

	if *transport == "ssh" || *transport == "both" {
		r := benchSSH(sshAddr, *user, *pass, *iterations, *concurrency, *commands, *profile, rttMs)
		results = append(results, r...)
	}
	if *transport == "https" || *transport == "both" {
		r := benchHTTPS(httpsAddr, *user, *pass, *iterations, *concurrency, *commands, *profile, rttMs)
		results = append(results, r...)
	}
	if *transport == "proxy" || *transport == "both" {
		r := benchProxy(proxyAddr, proxyPooledAddr, *user, *pass, *iterations, *concurrency, *commands, *profile, rttMs)
		results = append(results, r...)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(results)
}

func sshConfig(user, pass string) *ssh.ClientConfig {
	return &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.Password(pass)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         30 * time.Second,
	}
}

func benchSSH(addr, user, pass string, iterations, concurrency, cmdsPerIter int, profile string, rttMs float64) []Result {
	log.Printf("Benchmarking SSH (%d iterations, %d concurrency, %d cmds/iter)", iterations, concurrency, cmdsPerIter)
	cfg := sshConfig(user, pass)

	// Mode 1: fresh connection per iteration
	freshTimes := runParallel(iterations, concurrency, func() time.Duration {
		start := time.Now()
		conn, err := ssh.Dial("tcp", addr, cfg)
		if err != nil {
			log.Printf("ssh dial: %v", err)
			return errDuration
		}
		defer conn.Close()
		for i := 0; i < cmdsPerIter; i++ {
			sess, err := conn.NewSession()
			if err != nil {
				log.Printf("ssh session: %v", err)
				return errDuration
			}
			_, err = sess.Output("show version")
			sess.Close()
			if err != nil {
				log.Printf("ssh exec: %v", err)
				return errDuration
			}
		}
		return time.Since(start)
	})

	// Mode 2: reuse one connection (ControlMaster-style)
	// Warmup: establish connection + one throwaway iteration
	sharedConn, err := ssh.Dial("tcp", addr, cfg)
	var reuseTimes []time.Duration
	if err != nil {
		log.Printf("ssh reuse dial: %v (skipping reuse test)", err)
	} else {
		if sess, err := sharedConn.NewSession(); err == nil {
			sess.Output("show version")
			sess.Close()
		}
		reuseTimes = runParallel(iterations, concurrency, func() time.Duration {
			start := time.Now()
			for i := 0; i < cmdsPerIter; i++ {
				sess, err := sharedConn.NewSession()
				if err != nil {
					log.Printf("ssh reuse session: %v", err)
					return errDuration
				}
				_, err = sess.Output("show version")
				sess.Close()
				if err != nil {
					log.Printf("ssh reuse exec: %v", err)
					return errDuration
				}
			}
			return time.Since(start)
		})
		sharedConn.Close()
	}

	// Mode 3: batch exec — send multi-line payload over a single exec session
	batchPayload := generateExecPayload(cmdsPerIter)
	batchTimes := runParallel(iterations, concurrency, func() time.Duration {
		start := time.Now()
		conn, err := ssh.Dial("tcp", addr, cfg)
		if err != nil {
			log.Printf("ssh batch dial: %v", err)
			return errDuration
		}
		defer conn.Close()
		sess, err := conn.NewSession()
		if err != nil {
			log.Printf("ssh batch session: %v", err)
			return errDuration
		}
		_, err = sess.Output(batchPayload)
		sess.Close()
		if err != nil {
			log.Printf("ssh batch exec: %v", err)
			return errDuration
		}
		return time.Since(start)
	})

	results := []Result{
		summarize("ssh", "fresh-conn", cmdsPerIter, iterations, concurrency, profile, rttMs, freshTimes),
	}
	if reuseTimes != nil {
		results = append(results, summarize("ssh", "reuse-conn", cmdsPerIter, iterations, concurrency, profile, rttMs, reuseTimes))
	}
	results = append(results, summarize("ssh", "batch-exec", cmdsPerIter, iterations, concurrency, profile, rttMs, batchTimes))
	return results
}

func benchHTTPS(addr, user, pass string, iterations, concurrency, cmdsPerIter int, profile string, rttMs float64) []Result {
	log.Printf("Benchmarking HTTPS (%d iterations, %d concurrency, %d cmds/iter)", iterations, concurrency, cmdsPerIter)

	tlsCfg := &tls.Config{InsecureSkipVerify: true}

	// Mode 1: fresh connection per iteration (DisableKeepAlives)
	freshTimes := runParallel(iterations, concurrency, func() time.Duration {
		start := time.Now()
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig:   tlsCfg,
				DisableKeepAlives: true,
			},
			Timeout: 30 * time.Second,
		}
		for i := 0; i < cmdsPerIter; i++ {
			if err := doHTTPExec(client, addr, user, pass); err != nil {
				log.Printf("https fresh: %v", err)
				return errDuration
			}
		}
		return time.Since(start)
	})

	// Mode 2: keep-alive — shared client across ALL iterations
	keepAliveClient := &http.Client{
		Transport: &http.Transport{TLSClientConfig: tlsCfg},
		Timeout:   30 * time.Second,
	}
	// Warmup: one request to establish the TLS session
	doHTTPExec(keepAliveClient, addr, user, pass)

	reuseTimes := runParallel(iterations, concurrency, func() time.Duration {
		start := time.Now()
		for i := 0; i < cmdsPerIter; i++ {
			if err := doHTTPExec(keepAliveClient, addr, user, pass); err != nil {
				log.Printf("https keep-alive: %v", err)
				return errDuration
			}
		}
		return time.Since(start)
	})

	// Mode 3: batch POST — all commands in one request body
	batchPayload := generateExecPayload(cmdsPerIter)
	batchTimes := runParallel(iterations, concurrency, func() time.Duration {
		start := time.Now()
		client := &http.Client{
			Transport: &http.Transport{TLSClientConfig: tlsCfg},
			Timeout:   30 * time.Second,
		}
		url := fmt.Sprintf("https://%s/admin/config", addr)
		req, err := http.NewRequest("POST", url, strings.NewReader(batchPayload))
		if err != nil {
			return errDuration
		}
		req.SetBasicAuth(user, pass)
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("https batch: %v", err)
			return errDuration
		}
		io.ReadAll(resp.Body)
		resp.Body.Close()
		return time.Since(start)
	})

	results := []Result{
		summarize("https", "fresh-conn", cmdsPerIter, iterations, concurrency, profile, rttMs, freshTimes),
		summarize("https", "keep-alive", cmdsPerIter, iterations, concurrency, profile, rttMs, reuseTimes),
		summarize("https", "batch-post", cmdsPerIter, iterations, concurrency, profile, rttMs, batchTimes),
	}

	// Mode 4: multi-command GET (ASA slash syntax) — only if >1 cmd
	if cmdsPerIter > 1 {
		cmdParts := make([]string, cmdsPerIter)
		for i := range cmdParts {
			cmdParts[i] = "show+version"
		}
		multiPath := strings.Join(cmdParts, "/")

		multiTimes := runParallel(iterations, concurrency, func() time.Duration {
			start := time.Now()
			client := &http.Client{
				Transport: &http.Transport{TLSClientConfig: tlsCfg},
				Timeout:   30 * time.Second,
			}
			url := fmt.Sprintf("https://%s/admin/exec/%s", addr, multiPath)
			req, err := http.NewRequest("GET", url, nil)
			if err != nil {
				return errDuration
			}
			req.SetBasicAuth(user, pass)
			resp, err := client.Do(req)
			if err != nil {
				log.Printf("https multi: %v", err)
				return errDuration
			}
			io.ReadAll(resp.Body)
			resp.Body.Close()
			return time.Since(start)
		})
		results = append(results, summarize("https", "multi-cmd", cmdsPerIter, iterations, concurrency, profile, rttMs, multiTimes))
	}

	return results
}

func benchProxy(freshAddr, pooledAddr, user, pass string, iterations, concurrency, cmdsPerIter int, profile string, rttMs float64) []Result {
	log.Printf("Benchmarking Proxy (%d iterations, %d concurrency, %d cmds/iter)", iterations, concurrency, cmdsPerIter)

	tlsCfg := &tls.Config{InsecureSkipVerify: true}
	payload := generateExecPayload(cmdsPerIter)

	doProxy := func(addr string) time.Duration {
		start := time.Now()
		client := &http.Client{
			Transport: &http.Transport{TLSClientConfig: tlsCfg},
			Timeout:   30 * time.Second,
		}
		url := fmt.Sprintf("https://%s/admin/config", addr)
		req, err := http.NewRequest("POST", url, strings.NewReader(payload))
		if err != nil {
			return errDuration
		}
		req.SetBasicAuth(user, pass)
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("proxy: %v", err)
			return errDuration
		}
		io.ReadAll(resp.Body)
		resp.Body.Close()
		return time.Since(start)
	}

	freshTimes := runParallel(iterations, concurrency, func() time.Duration { return doProxy(freshAddr) })
	pooledTimes := runParallel(iterations, concurrency, func() time.Duration { return doProxy(pooledAddr) })

	return []Result{
		summarize("proxy", "fresh-ssh", cmdsPerIter, iterations, concurrency, profile, rttMs, freshTimes),
		summarize("proxy", "pooled-ssh", cmdsPerIter, iterations, concurrency, profile, rttMs, pooledTimes),
	}
}

// doHTTPExec sends a single show+version GET and drains the response.
func doHTTPExec(client *http.Client, addr, user, pass string) error {
	url := fmt.Sprintf("https://%s/admin/exec/show+version", addr)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(user, pass)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()
	return nil
}

// generateExecPayload creates a newline-delimited string of N show commands.
func generateExecPayload(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "show version\n")
	}
	return b.String()
}

func runParallel(iterations, concurrency int, fn func() time.Duration) []time.Duration {
	results := make([]time.Duration, iterations)
	sem := make(chan struct{}, concurrency)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < iterations; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int) {
			defer wg.Done()
			defer func() { <-sem }()
			d := fn()
			mu.Lock()
			results[idx] = d
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	return results
}

func summarize(transport, op string, cmds, iterations, concurrency int, profile string, rttMs float64, times []time.Duration) Result {
	// Filter out errors (errDuration sentinel)
	valid := make([]float64, 0, len(times))
	errors := 0
	for _, t := range times {
		if t == errDuration {
			errors++
			continue
		}
		valid = append(valid, float64(t.Microseconds())/1000)
	}
	if errors > 0 {
		log.Printf("  %s/%s: %d/%d iterations failed", transport, op, errors, iterations)
	}
	if len(valid) == 0 {
		return Result{
			Transport: transport, Operation: op, Commands: cmds,
			Iterations: iterations, Errors: errors, Concurrency: concurrency,
			Latency: profile, RTTms: rttMs,
		}
	}

	sort.Float64s(valid)
	n := len(valid)

	var sum float64
	for _, v := range valid {
		sum += v
	}
	avg := sum / float64(n)

	var variance float64
	for _, v := range valid {
		d := v - avg
		variance += d * d
	}
	stddev := math.Sqrt(variance / float64(n))

	return Result{
		Transport:   transport,
		Operation:   op,
		Commands:    cmds,
		Iterations:  iterations,
		Errors:      errors,
		Concurrency: concurrency,
		Latency:     profile,
		RTTms:       rttMs,
		AvgMs:       avg,
		MinMs:       valid[0],
		MaxMs:       valid[n-1],
		P50Ms:       percentile(valid, 50),
		P95Ms:       percentile(valid, 95),
		StddevMs:    stddev,
	}
}

// percentile returns the p-th percentile from a sorted slice.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	rank := (p / 100) * float64(len(sorted)-1)
	lower := int(rank)
	upper := lower + 1
	if upper >= len(sorted) {
		return sorted[len(sorted)-1]
	}
	frac := rank - float64(lower)
	return sorted[lower] + frac*(sorted[upper]-sorted[lower])
}

package main

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
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

type Result struct {
	Transport   string  `json:"transport"`
	Operation   string  `json:"operation"`
	Commands    int     `json:"commands"`
	Iterations  int     `json:"iterations"`
	Concurrency int     `json:"concurrency"`
	Latency     string  `json:"latency_profile"`
	RTTms       float64 `json:"simulated_rtt_ms"`
	TotalMs     float64 `json:"total_ms"`
	AvgMs       float64 `json:"avg_ms"`
	MinMs       float64 `json:"min_ms"`
	MaxMs       float64 `json:"max_ms"`
}

// Latency profiles sourced from Verizon Enterprise monthly backbone
// measurements (March 2026) and AWS/RIPE Atlas data.
// See plans/ssh-vs-https-cli/latency-profiles.md for full citations.
var latencyProfiles = map[string]time.Duration{
	// 0ms added — baseline, co-located automation server
	"local": 0,
	// 1ms one-way (2ms RTT) — intra-DC / campus LAN
	// Ref: AWS intra-AZ <1ms; Prisma 2024 report p50 1-2ms intra-region
	"campus": 1 * time.Millisecond,
	// 15ms one-way (30ms RTT) — intra-country / single region
	// Ref: Verizon Mar 2026: US Private IP 29.9ms, Europe 15.2ms RTT
	"regional": 15 * time.Millisecond,
	// 35ms one-way (70ms RTT) — cross-continent / transatlantic
	// Ref: Verizon Mar 2026: Transatlantic 70.2ms RTT (SLA ≤90ms)
	"continental": 35 * time.Millisecond,
	// 75ms one-way (150ms RTT) — US ↔ East Asia
	// Ref: Verizon Mar 2026: HK-to-US 145.5ms RTT (SLA ≤230ms)
	"intercontinental": 75 * time.Millisecond,
	// 87ms one-way (175ms RTT) — US ↔ Australia/NZ
	// Ref: Verizon Mar 2026: NZ Transpacific 174.2ms RTT (SLA ≤210ms)
	"transpacific": 87 * time.Millisecond,
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
		log.Fatalf("unknown latency profile %q (options: local, campus, regional, continental, intercontinental, transpacific)", *profile)
	}
	rttMs := float64(delay.Milliseconds()) * 2

	sshAddr := fmt.Sprintf("localhost:%d", *sshPort)
	httpsAddr := fmt.Sprintf("localhost:%d", *httpsPort)

	// Always embedded — start server with latency injection
	dev, err := device.New("bench-rtr", *user, *pass, *transcriptsDir)
	if err != nil {
		log.Fatalf("device: %v", err)
	}

	sshLn, err := net.Listen("tcp", sshAddr)
	if err != nil {
		log.Fatalf("ssh listen: %v", err)
	}
	httpsLn, err := net.Listen("tcp", httpsAddr)
	if err != nil {
		log.Fatalf("https listen: %v", err)
	}

	// Wrap listeners with latency injection
	sshSrv, err := sshserver.New(sshAddr, dev)
	if err != nil {
		log.Fatalf("ssh: %v", err)
	}
	sshSrv.SetListener(&latencyPkg.Listener{Listener: sshLn, Delay: delay})
	go sshSrv.ListenAndServe()

	httpSrv := httpserver.New(httpsAddr, dev)
	httpSrv.SetListener(&latencyPkg.Listener{Listener: httpsLn, Delay: delay})
	go httpSrv.ListenAndServeTLS()

	// Proxy: HTTPS frontend with WAN latency → SSH backend with campus latency (1ms one-way)
	proxyAddr := fmt.Sprintf("localhost:%d", *proxyPort)
	// Backend SSH device gets campus-level latency (1ms one-way = 2ms RTT)
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
	campusDelay := 1 * time.Millisecond
	backendSrv.SetListener(&latencyPkg.Listener{Listener: backendLn, Delay: campusDelay})
	go backendSrv.ListenAndServe()

	// Proxy HTTPS listener gets WAN latency
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
	if delay > 0 {
		log.Printf("Server ready — profile=%s, simulated RTT=%.0fms", *profile, rttMs)
	} else {
		log.Printf("Server ready — profile=local (no added latency)")
	}

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

func benchSSH(addr, user, pass string, iterations, concurrency, cmdsPerIter int, profile string, rttMs float64) []Result {
	log.Printf("Benchmarking SSH (%d iterations, %d concurrency, %d cmds/iter)", iterations, concurrency, cmdsPerIter)

	sshCfg := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.Password(pass)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         30 * time.Second,
	}

	// Mode 1: fresh connection per iteration
	freshTimes := runParallel(iterations, concurrency, func() time.Duration {
		start := time.Now()
		conn, err := ssh.Dial("tcp", addr, sshCfg)
		if err != nil {
			log.Printf("ssh dial: %v", err)
			return 0
		}
		defer conn.Close()
		for i := 0; i < cmdsPerIter; i++ {
			sess, err := conn.NewSession()
			if err != nil {
				log.Printf("ssh session: %v", err)
				return 0
			}
			out, err := sess.Output("show version")
			_ = out
			sess.Close()
			if err != nil {
				log.Printf("ssh exec: %v", err)
			}
		}
		return time.Since(start)
	})

	// Mode 2: reuse one connection across all iterations (ControlMaster-style)
	// Open one SSH conn, then each iteration just opens a new exec session on it.
	sharedConn, err := ssh.Dial("tcp", addr, sshCfg)
	var reuseTimes []time.Duration
	if err != nil {
		log.Printf("ssh reuse dial: %v (skipping reuse test)", err)
	} else {
		reuseTimes = runParallel(iterations, concurrency, func() time.Duration {
			start := time.Now()
			for i := 0; i < cmdsPerIter; i++ {
				sess, err := sharedConn.NewSession()
				if err != nil {
					log.Printf("ssh reuse session: %v", err)
					return 0
				}
				out, err := sess.Output("show version")
				_ = out
				sess.Close()
				if err != nil {
					log.Printf("ssh reuse exec: %v", err)
				}
			}
			return time.Since(start)
		})
		sharedConn.Close()
	}

	// Mode 3: config push — send multi-line config over a single exec session
	configLines := generateConfigBlock(cmdsPerIter)
	configTimes := runParallel(iterations, concurrency, func() time.Duration {
		start := time.Now()
		conn, err := ssh.Dial("tcp", addr, sshCfg)
		if err != nil {
			log.Printf("ssh config dial: %v", err)
			return 0
		}
		defer conn.Close()
		sess, err := conn.NewSession()
		if err != nil {
			log.Printf("ssh config session: %v", err)
			return 0
		}
		out, err := sess.Output(configLines)
		_ = out
		sess.Close()
		if err != nil {
			log.Printf("ssh config exec: %v", err)
		}
		return time.Since(start)
	})

	results := []Result{
		summarize("ssh", "fresh-conn", cmdsPerIter, iterations, concurrency, profile, rttMs, freshTimes),
	}
	if reuseTimes != nil {
		results = append(results, summarize("ssh", "reuse-conn", cmdsPerIter, iterations, concurrency, profile, rttMs, reuseTimes))
	}
	results = append(results, summarize("ssh", "config-push", cmdsPerIter, iterations, concurrency, profile, rttMs, configTimes))
	return results
}

func benchHTTPS(addr, user, pass string, iterations, concurrency, cmdsPerIter int, profile string, rttMs float64) []Result {
	log.Printf("Benchmarking HTTPS (%d iterations, %d concurrency, %d cmds/iter)", iterations, concurrency, cmdsPerIter)

	// Benchmark 1: fresh connection per iteration
	freshTimes := runParallel(iterations, concurrency, func() time.Duration {
		start := time.Now()
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
				DisableKeepAlives: true,
			},
			Timeout: 30 * time.Second,
		}

		for i := 0; i < cmdsPerIter; i++ {
			url := fmt.Sprintf("https://%s/admin/exec/show+version", addr)
			req, _ := http.NewRequest("GET", url, nil)
			req.SetBasicAuth(user, pass)
			resp, err := client.Do(req)
			if err != nil {
				log.Printf("https: %v", err)
				return 0
			}
			io.ReadAll(resp.Body)
			resp.Body.Close()
		}
		return time.Since(start)
	})

	// Benchmark 2: reused connection (keep-alive)
	reuseTimes := runParallel(iterations, concurrency, func() time.Duration {
		start := time.Now()
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
			Timeout: 30 * time.Second,
		}

		for i := 0; i < cmdsPerIter; i++ {
			url := fmt.Sprintf("https://%s/admin/exec/show+version", addr)
			req, _ := http.NewRequest("GET", url, nil)
			req.SetBasicAuth(user, pass)
			resp, err := client.Do(req)
			if err != nil {
				log.Printf("https: %v", err)
				return 0
			}
			io.ReadAll(resp.Body)
			resp.Body.Close()
		}
		return time.Since(start)
	})

	// Benchmark 3: config push (POST /admin/config) — single request, all commands
	configBody := generateConfigBlock(cmdsPerIter)
	configTimes := runParallel(iterations, concurrency, func() time.Duration {
		start := time.Now()
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
			Timeout: 30 * time.Second,
		}
		url := fmt.Sprintf("https://%s/admin/config", addr)
		req, _ := http.NewRequest("POST", url, strings.NewReader(configBody))
		req.SetBasicAuth(user, pass)
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("https config: %v", err)
			return 0
		}
		io.ReadAll(resp.Body)
		resp.Body.Close()
		return time.Since(start)
	})

	// Benchmark 4: multi-command in single GET (ASA slash syntax) — only if >1 cmd
	if cmdsPerIter > 1 {
		cmdParts := make([]string, cmdsPerIter)
		for i := range cmdParts {
			cmdParts[i] = "show+version"
		}
		multiPath := strings.Join(cmdParts, "/")

		multiTimes := runParallel(iterations, concurrency, func() time.Duration {
			start := time.Now()
			client := &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
				},
				Timeout: 30 * time.Second,
			}
			url := fmt.Sprintf("https://%s/admin/exec/%s", addr, multiPath)
			req, _ := http.NewRequest("GET", url, nil)
			req.SetBasicAuth(user, pass)
			resp, err := client.Do(req)
			if err != nil {
				log.Printf("https: %v", err)
				return 0
			}
			io.ReadAll(resp.Body)
			resp.Body.Close()
			return time.Since(start)
		})

		return []Result{
			summarize("https", "fresh-conn", cmdsPerIter, iterations, concurrency, profile, rttMs, freshTimes),
			summarize("https", "keep-alive", cmdsPerIter, iterations, concurrency, profile, rttMs, reuseTimes),
			summarize("https", "multi-cmd", cmdsPerIter, iterations, concurrency, profile, rttMs, multiTimes),
			summarize("https", "config-push", cmdsPerIter, iterations, concurrency, profile, rttMs, configTimes),
		}
	}

	return []Result{
		summarize("https", "fresh-conn", cmdsPerIter, iterations, concurrency, profile, rttMs, freshTimes),
		summarize("https", "keep-alive", cmdsPerIter, iterations, concurrency, profile, rttMs, reuseTimes),
		summarize("https", "config-push", cmdsPerIter, iterations, concurrency, profile, rttMs, configTimes),
	}
}

func benchProxy(freshAddr, pooledAddr, user, pass string, iterations, concurrency, cmdsPerIter int, profile string, rttMs float64) []Result {
	log.Printf("Benchmarking Proxy (%d iterations, %d concurrency, %d cmds/iter)", iterations, concurrency, cmdsPerIter)

	// Proxy with fresh SSH to backend per request
	freshTimes := runParallel(iterations, concurrency, func() time.Duration {
		start := time.Now()
		configBody := generateConfigBlock(cmdsPerIter)
		client := &http.Client{
			Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
			Timeout:   30 * time.Second,
		}
		url := fmt.Sprintf("https://%s/admin/config", freshAddr)
		req, _ := http.NewRequest("POST", url, strings.NewReader(configBody))
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("proxy fresh: %v", err)
			return 0
		}
		io.ReadAll(resp.Body)
		resp.Body.Close()
		return time.Since(start)
	})

	// Proxy with pooled SSH to backend
	pooledTimes := runParallel(iterations, concurrency, func() time.Duration {
		start := time.Now()
		configBody := generateConfigBlock(cmdsPerIter)
		client := &http.Client{
			Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
			Timeout:   30 * time.Second,
		}
		url := fmt.Sprintf("https://%s/admin/config", pooledAddr)
		req, _ := http.NewRequest("POST", url, strings.NewReader(configBody))
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("proxy pooled: %v", err)
			return 0
		}
		io.ReadAll(resp.Body)
		resp.Body.Close()
		return time.Since(start)
	})

	return []Result{
		summarize("proxy", "fresh-ssh", cmdsPerIter, iterations, concurrency, profile, rttMs, freshTimes),
		summarize("proxy", "pooled-ssh", cmdsPerIter, iterations, concurrency, profile, rttMs, pooledTimes),
	}
}

// generateConfigBlock creates a newline-delimited string of N show commands,
// simulating a config push payload.
func generateConfigBlock(n int) string {
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
	var total, min, max time.Duration
	min = time.Hour
	for _, t := range times {
		total += t
		if t < min {
			min = t
		}
		if t > max {
			max = t
		}
	}
	avg := total / time.Duration(len(times))
	return Result{
		Transport:   transport,
		Operation:   op,
		Commands:    cmds,
		Iterations:  iterations,
		Concurrency: concurrency,
		Latency:     profile,
		RTTms:       rttMs,
		TotalMs:     float64(total.Microseconds()) / 1000,
		AvgMs:       float64(avg.Microseconds()) / 1000,
		MinMs:       float64(min.Microseconds()) / 1000,
		MaxMs:       float64(max.Microseconds()) / 1000,
	}
}

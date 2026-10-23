// attest-load fires K concurrent POST /v1/attestation requests at a helper (or
// the AWS queue) and prints each request's status and latency. It never prints
// the token or a proof: only an 8-hex-digit hash so repeated proofs show up.
//
//	attest-load -url http://127.0.0.1:19086 -token-file ~/path/token -k 10 -rounds 3
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/liveattestation"
)

type result struct {
	index   int
	status  int
	latency time.Duration
	proof   string
	err     string
}

func main() {
	target := flag.String("url", "http://127.0.0.1:19086", "helper origin (never the live 19085 for K>1)")
	tokenFile := flag.String("token-file", "", "owner-only helper token file (same as the helper's)")
	k := flag.Int("k", 1, "concurrent requests per round")
	rounds := flag.Int("rounds", 1, "rounds")
	gap := flag.Duration("gap", 2*time.Second, "pause between rounds")
	timeout := flag.Duration("timeout", 8*time.Second, "per-request timeout (the gateway uses 8 s)")
	flag.Parse()
	if *k < 1 || *k > 64 || *rounds < 1 {
		fail("k must be 1-64 and rounds >= 1")
	}
	token, err := liveattestation.ReadHelperTokenFile(*tokenFile)
	if err != nil {
		fail(err.Error())
	}
	base := strings.TrimRight(*target, "/")
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.MaxIdleConnsPerHost = 64
	client := &http.Client{Timeout: *timeout, Transport: transport}

	var all []result
	proofs := map[string]int{}
	for round := 1; round <= *rounds; round++ {
		results := make([]result, *k)
		var wg sync.WaitGroup
		start := make(chan struct{})
		for i := range *k {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				results[i] = fire(client, base, token, i)
			}()
		}
		began := time.Now()
		close(start)
		wg.Wait()
		wall := time.Since(began)
		sort.Slice(results, func(a, b int) bool { return results[a].latency < results[b].latency })
		fmt.Printf("round %d  k=%d  wall %s\n", round, *k, ms(wall))
		for _, r := range results {
			line := fmt.Sprintf("  req %2d  HTTP %d  %8s", r.index, r.status, ms(r.latency))
			if r.proof != "" {
				line += "  proof#" + r.proof
				proofs[r.proof]++
			}
			if r.err != "" {
				line += "  " + r.err
			}
			fmt.Println(line)
		}
		all = append(all, results...)
		if round < *rounds {
			time.Sleep(*gap)
		}
	}
	summarize(all, proofs)
}

func fire(client *http.Client, base, token string, index int) result {
	r := result{index: index}
	request, err := http.NewRequest(http.MethodPost, base+"/v1/attestation", nil)
	if err != nil {
		r.err = "bad url"
		return r
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/json")
	started := time.Now()
	response, err := client.Do(request)
	if err != nil {
		r.latency = time.Since(started)
		r.err = "request failed or timed out"
		return r
	}
	body, _ := io.ReadAll(io.LimitReader(response.Body, 32*1024))
	_ = response.Body.Close()
	r.latency = time.Since(started)
	r.status = response.StatusCode
	var payload struct {
		Attestation string `json:"attestation"`
		Error       string `json:"error"`
	}
	_ = json.Unmarshal(body, &payload)
	if payload.Attestation != "" {
		sum := sha256.Sum256([]byte(payload.Attestation))
		r.proof = hex.EncodeToString(sum[:4])
	} else if payload.Error != "" {
		r.err = payload.Error // helper errors are fixed strings, never secrets
	}
	return r
}

func summarize(all []result, proofs map[string]int) {
	var ok []time.Duration
	codes := map[int]int{}
	for _, r := range all {
		codes[r.status]++
		if r.status == http.StatusOK {
			ok = append(ok, r.latency)
		}
	}
	sort.Slice(ok, func(a, b int) bool { return ok[a] < ok[b] })
	repeated := 0
	for _, n := range proofs {
		if n > 1 {
			repeated += n
		}
	}
	fmt.Printf("summary: %d requests, status counts %v, repeated proofs %d\n", len(all), codes, repeated)
	if len(ok) > 0 {
		fmt.Printf("  200 latency: min %s  median %s  p90 %s  max %s\n",
			ms(ok[0]), ms(ok[len(ok)/2]), ms(ok[(len(ok)*9)/10]), ms(ok[len(ok)-1]))
	}
}

func ms(d time.Duration) string { return fmt.Sprintf("%.0fms", float64(d.Microseconds())/1000) }

func fail(message string) {
	fmt.Fprintln(os.Stderr, "attest-load:", message)
	os.Exit(2)
}

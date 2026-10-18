package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/liveattestation"
)

func main() {
	generate := flag.Bool("generate", false, "generate a real proof; print only validation result")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	provider := liveattestation.NewProvider()
	err := provider.Check(ctx)
	generated := false
	if err == nil && *generate {
		_, err = provider.Generate(ctx)
		generated = err == nil
	}
	result := map[string]any{"os": runtime.GOOS, "arch": runtime.GOARCH, "supported": err == nil, "generated": generated}
	if err != nil {
		result["error"] = err.Error()
	}
	_ = json.NewEncoder(os.Stdout).Encode(result)
	if err != nil {
		fmt.Fprintln(os.Stderr, "attestation probe failed")
		os.Exit(1)
	}
}

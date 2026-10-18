package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/liveattestation"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:19085", "loopback listener")
	tokenFile := flag.String("token-file", "", "owner-only helper token file")
	flag.Parse()
	host, _, err := net.SplitHostPort(*listen)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		log.Fatal("helper must listen on a loopback IP")
	}
	token, err := liveattestation.ReadHelperTokenFile(*tokenFile)
	if err != nil {
		log.Fatal(err)
	}
	provider := liveattestation.NewLocalProvider()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	err = provider.Check(ctx)
	cancel()
	if err != nil {
		log.Fatal("local attestation runtime unavailable")
	}
	handler, err := liveattestation.NewHelperHandler(provider, token)
	if err != nil {
		log.Fatal(err)
	}
	server := &http.Server{Addr: *listen, Handler: handler, ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout: 3 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 15 * time.Second, MaxHeaderBytes: 4096}
	stop, stopCancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopCancel()
	go func() {
		<-stop.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()
	log.Printf("attestation helper listening on %s (local-only; no token logging)", *listen)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

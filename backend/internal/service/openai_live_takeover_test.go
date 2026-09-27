package service

import (
	"testing"
	"time"
)

func TestLiveTakeoverWithoutObserverKeepsPollWait(t *testing.T) {
	started := time.Now()
	awaitLiveObserverRelease("no-observer-call")
	if elapsed := time.Since(started); elapsed < liveObserverPollInterval {
		t.Fatalf("waited %s, want the poll interval %s", elapsed, liveObserverPollInterval)
	}
}

func TestLiveTakeoverHandsOverWithoutPollWait(t *testing.T) {
	takeover := registerLiveTakeover("call-a")
	go func() {
		<-takeover.requested
		time.Sleep(10 * time.Millisecond) // observer closes its connection
		takeover.release("call-a")
	}()
	started := time.Now()
	awaitLiveObserverRelease("call-a")
	if elapsed := time.Since(started); elapsed >= liveObserverPollInterval {
		t.Fatalf("handover took %s, want well under %s", elapsed, liveObserverPollInterval)
	}
	if _, ok := liveTakeovers.Load("call-a"); ok {
		t.Fatal("released takeover is still registered")
	}
}

func TestLiveTakeoverOldObserverCannotRemoveNewOne(t *testing.T) {
	old := registerLiveTakeover("call-b")
	fresh := registerLiveTakeover("call-b")
	old.release("call-b")
	value, ok := liveTakeovers.Load("call-b")
	if !ok || value.(*liveTakeover) != fresh {
		t.Fatal("an old observer's release removed the new observer's takeover")
	}
	fresh.release("call-b")
}

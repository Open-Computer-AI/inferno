package service

import (
	"sync"
	"time"
)

// liveTakeover lets a sideband proxy take a call over from the observer running
// in this process without waiting a full poll interval: the proxy requests it,
// the observer aborts its dial or closes its connection and confirms. An
// observer in another gateway instance is not registered here, so the proxy
// keeps the poll-interval wait for it.
type liveTakeover struct {
	requested   chan struct{}
	released    chan struct{}
	requestOnce sync.Once
	releaseOnce sync.Once
}

// liveTakeovers maps a call hash to the takeover of its current observer.
var liveTakeovers sync.Map

// registerLiveTakeover replaces any entry for the call: a new observer (after a
// proxy disconnect) must not inherit a previous observer's closed channels.
func registerLiveTakeover(callHash string) *liveTakeover {
	takeover := &liveTakeover{requested: make(chan struct{}), released: make(chan struct{})}
	liveTakeovers.Store(callHash, takeover)
	return takeover
}

func (t *liveTakeover) request() { t.requestOnce.Do(func() { close(t.requested) }) }

// release confirms the observer holds no upstream connection and removes its entry.
func (t *liveTakeover) release(callHash string) {
	t.releaseOnce.Do(func() { close(t.released) })
	liveTakeovers.CompareAndDelete(callHash, t)
}

// awaitLiveObserverRelease blocks until this process's observer of the call has
// let go, or for at most liveObserverPollInterval (the previous fixed wait).
func awaitLiveObserverRelease(callHash string) {
	value, ok := liveTakeovers.Load(callHash)
	if !ok {
		time.Sleep(liveObserverPollInterval)
		return
	}
	takeover := value.(*liveTakeover)
	takeover.request()
	timer := time.NewTimer(liveObserverPollInterval)
	defer timer.Stop()
	select {
	case <-takeover.released:
	case <-timer.C:
	}
}

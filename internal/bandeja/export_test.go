package bandeja

import (
	"net"
	"time"
)

// SetIdleForTest overrides the idle-shutdown timing; the returned func restores it.
func SetIdleForTest(tick, limit time.Duration) func() {
	oldTick, oldLimit := idleTick, idleLimit
	idleTick, idleLimit = tick, limit
	return func() { idleTick, idleLimit = oldTick, oldLimit }
}

// SetListenForTest overrides the listener factory; the returned func restores it.
func SetListenForTest(f func(network, addr string) (net.Listener, error)) func() {
	old := netListen
	netListen = f
	return func() { netListen = old }
}

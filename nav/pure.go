package nav

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	pmDest    = "io.github.rinigus.PureMaps"
	pmPath    = "/io/github/rinigus/PureMaps/navigator"
	pmIface   = "io.github.rinigus.PureMaps.navigator"
	pmTimeout = 2500 * time.Millisecond
	pollEvery = time.Second
)

// Scalar properties only. The route array is fetched on its own, and not
// on this one-second poll: a long trip is hundreds of kilobytes.
const fetchScript = `
dest=` + pmDest + `
path=` + pmPath + `
iface=` + pmIface + `
for p in hasRoute running alongRoute directionValid icon manDist manTime street narrative destDist destTime destEta direction progress roundaboutExit; do
  v=$(gdbus call --session --dest "$dest" --object-path "$path" --method org.freedesktop.DBus.Properties.Get "$iface" "$p" 2>/dev/null) || continue
  printf '%s\t%s\n' "$p" "$v"
done
`

const (
	routeEvery   = 10 * time.Second
	routeTimeout = 5 * time.Second
)

const routeScript = `gdbus call --session --dest ` + pmDest + ` --object-path ` + pmPath + ` --method org.freedesktop.DBus.Properties.Get ` + pmIface + ` route`

var (
	runFetch = fetchScalars
	runRoute = fetchRoute
)

type pureSource struct {
	mu      sync.Mutex
	trip    Trip
	route   []Point
	routeAt time.Time

	once sync.Once
	stop chan struct{}
	done chan struct{}
}

func openPureMaps() Source {
	s := &pureSource{
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	go s.loop()
	return s
}

func (s *pureSource) loop() {
	defer close(s.done)
	s.poll()
	tick := time.NewTicker(pollEvery)
	defer tick.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-tick.C:
			s.poll()
		}
	}
}

func (s *pureSource) poll() {
	select {
	case <-s.stop:
		return
	default:
	}
	out, err := runFetch()
	trip := Trip{Err: ErrNotRunning}
	if err == nil {
		trip = parseFetch(out)
		if !trip.OK {
			trip.Err = ErrNotRunning
		}
	}
	fetchPath := false
	s.mu.Lock()
	s.trip = trip
	if !trip.HasRoute {
		s.route = nil
		s.routeAt = time.Time{}
	} else if s.routeAt.IsZero() || time.Since(s.routeAt) >= routeEvery {
		s.routeAt = time.Now()
		fetchPath = true
	}
	s.mu.Unlock()
	if fetchPath {
		s.loadRoute()
	}
}

func (s *pureSource) loadRoute() {
	out, err := runRoute()
	if err != nil {
		return
	}
	pts := parseRoute(out)
	s.mu.Lock()
	if s.trip.HasRoute {
		s.route = pts
	}
	s.mu.Unlock()
}

func (s *pureSource) Route() []Point {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.route
}

func (s *pureSource) Snapshot() Trip {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.trip
}

func (s *pureSource) Close() {
	s.once.Do(func() { close(s.stop) })
	<-s.done
}

func fetchRoute() (string, error) {
	return runCmd(routeTimeout, "sh", "-c", routeScript)
}

func fetchScalars() (string, error) {
	return runCmd(pmTimeout, "sh", "-c", fetchScript)
}

func runCmd(limit time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = sessionBusEnv()
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	out := buf.String()
	if ctx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("gdbus timed out")
	}
	if err != nil {
		return out, err
	}
	return out, nil
}

func sessionBusEnv() []string {
	env := os.Environ()
	if strings.TrimSpace(os.Getenv("DBUS_SESSION_BUS_ADDRESS")) != "" {
		return env
	}
	candidates := []string{
		fmt.Sprintf("/run/user/%d/dbus/user_bus_socket", os.Getuid()),
		"/run/user/100000/dbus/user_bus_socket",
	}
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		candidates = append([]string{xdg + "/dbus/user_bus_socket"}, candidates...)
	}
	for _, sock := range candidates {
		if _, err := os.Stat(sock); err == nil {
			return append(env, "DBUS_SESSION_BUS_ADDRESS=unix:path="+sock)
		}
	}
	return env
}

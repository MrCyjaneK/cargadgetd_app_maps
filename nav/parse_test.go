package nav

import (
	"os"
	"path/filepath"
	"testing"
)

const sampleFetch = "hasRoute\t(<true>,)\n" +
	"running\t(<true>,)\n" +
	"alongRoute\t(<true>,)\n" +
	"directionValid\t(<true>,)\n" +
	"direction\t(<90.5>,)\n" +
	"icon\t(<'turn-right'>,)\n" +
	"manDist\t(<'200 m'>,)\n" +
	"manTime\t(<'30 s'>,)\n" +
	"street\t(<'High Street'>,)\n" +
	"narrative\t(<'Turn right onto High Street'>,)\n" +
	"destDist\t(<'12 km'>,)\n" +
	"destTime\t(<'18 min'>,)\n" +
	"destEta\t(<'15:42'>,)\n" +
	"progress\t(<40>,)\n" +
	"roundaboutExit\t(<0>,)\n"

func TestParseFetch(t *testing.T) {
	n := parseFetch(sampleFetch)
	if !n.OK || !n.HasRoute || !n.Running || !n.AlongRoute || !n.DirectionValid {
		t.Fatalf("%+v", n)
	}
	if n.Narrative != "Turn right onto High Street" || n.Street != "High Street" {
		t.Fatalf("text %+v", n)
	}
	if n.ManDist != "200 m" || n.ManTime != "30 s" || n.DestDist != "12 km" || n.DestTime != "18 min" || n.DestEta != "15:42" {
		t.Fatalf("dist %+v", n)
	}
	if n.Icon != "turn-right" || n.Progress != 40 || n.Direction != 90.5 {
		t.Fatalf("icon %+v", n)
	}
	exit := parseFetch("hasRoute\t(<true>,)\nroundaboutExit\t(<int32 2>,)\n")
	if !exit.OK || exit.RoundaboutExit != 2 {
		t.Fatalf("exit %+v", exit)
	}
	quoted := parseFetch("narrative\t(<'O\\'Brien'>,)\n")
	if quoted.Narrative != "O'Brien" {
		t.Fatalf("quote %q", quoted.Narrative)
	}
}

func TestParseFetchIdle(t *testing.T) {
	out := "hasRoute\t(<false>,)\ndestDist\t(<'-'>,)\ndestTime\t(<'-'>,)\nmanDist\t(<'-'>,)\nnarrative\t(<''>,)\n"
	n := parseFetch(out)
	if !n.OK || n.HasRoute || Plain(n.ManDist) != "" || Remain(n) != "" {
		t.Fatalf("%+v remain %q", n, Remain(n))
	}
}

func TestParseFetchEmpty(t *testing.T) {
	n := parseFetch("")
	if n.OK || n.HasRoute {
		t.Fatalf("%+v", n)
	}
}

func TestMockAndNormalize(t *testing.T) {
	if Normalize("mock") != "mock" || Normalize("PureMaps") != "puremaps" || Normalize("") != "puremaps" || Normalize("nope") != "puremaps" {
		t.Fatalf("normalize %q %q", Normalize("PureMaps"), Normalize("nope"))
	}
	tr := Open("mock").Snapshot()
	if !tr.HasRoute || tr.Icon != "turn-right" || tr.ManDist != "200 m" || tr.Street != "High Street" {
		t.Fatalf("mock %+v", tr)
	}
	if tr.Narrative != "Turn right onto High Street" || Remain(tr) != "18 min · 12 km" {
		t.Fatalf("mock line %+v %q", tr, Remain(tr))
	}
}

func TestPollError(t *testing.T) {
	prevF, prevR := runFetch, runRoute
	t.Cleanup(func() {
		runFetch = prevF
		runRoute = prevR
	})
	runFetch = func() (string, error) { return "", errStub }
	runRoute = func() (string, error) {
		t.Fatal("route fetched without a trip")
		return "", errStub
	}
	s := &pureSource{stop: make(chan struct{})}
	s.poll()
	tr := s.Snapshot()
	if tr.Err != ErrNotRunning || tr.HasRoute {
		t.Fatalf("%+v", tr)
	}
	runFetch = func() (string, error) { return sampleFetch, nil }
	runRoute = func() (string, error) { return "(<[50.1, 19.2, 50.2, 19.3]>,)", nil }
	s.poll()
	tr = s.Snapshot()
	if tr.Err != "" || !tr.HasRoute || tr.Street != "High Street" || !tr.Running {
		t.Fatalf("after poll %+v", tr)
	}
	pts := s.Route()
	if len(pts) != 2 || pts[0].Lat != 50.1 || pts[0].Lon != 19.2 || pts[1].Lat != 50.2 || pts[1].Lon != 19.3 {
		t.Fatalf("route %+v", pts)
	}
	calls := 0
	runRoute = func() (string, error) {
		calls++
		return "", errStub
	}
	s.poll()
	if calls != 0 {
		t.Fatalf("refetched route %d", calls)
	}
	runFetch = func() (string, error) { return "hasRoute\t(<false>,)\n", nil }
	s.poll()
	if s.Route() != nil {
		t.Fatal("route kept after clear")
	}
}

func TestParseRoute(t *testing.T) {
	pts := parseRoute("(<[50.25, 19.04, 50.30, 19.10, 48.14, 11.57]>,)")
	if len(pts) != 3 || pts[2].Lat != 48.14 || pts[2].Lon != 11.57 {
		t.Fatalf("%+v", pts)
	}
	if parseRoute("(<[1.0, 2.0, 3.0]>,)") != nil && len(parseRoute("(<[1.0, 2.0, 3.0]>,)")) != 1 {
		t.Fatal("odd tail")
	}
	odd := parseRoute("(<[1.5, 2.5, 9]>,)")
	if len(odd) != 1 || odd[0].Lat != 1.5 || odd[0].Lon != 2.5 {
		t.Fatalf("odd %+v", odd)
	}
	if parseRoute("(<@ad []>,)") != nil || parseRoute("") != nil {
		t.Fatal("empty route")
	}
}

func TestGlyphPathBothDirs(t *testing.T) {
	qml := t.TempDir()
	pm := t.TempDir()
	prev := GlyphDirs
	GlyphDirs = []string{qml, pm}
	t.Cleanup(func() { GlyphDirs = prev })

	if GlyphPath("") != "" || GlyphPath("../turn") != "" || GlyphPath("a/b") != "" {
		t.Fatal("unsafe name accepted")
	}
	if GlyphPath("turn-right") != "" {
		t.Fatal("missing icon found")
	}
	pmFile := filepath.Join(pm, "turn-right-white.svg")
	if err := os.WriteFile(pmFile, []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := GlyphPath("turn-right"); got != pmFile {
		t.Fatalf("pm %q", got)
	}
	qmlFile := filepath.Join(qml, "turn-right-white.svg")
	if err := os.WriteFile(qmlFile, []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := GlyphPath("turn-right"); got != qmlFile {
		t.Fatalf("qml wins %q", got)
	}
	plain := filepath.Join(pm, "flag.svg")
	if err := os.WriteFile(plain, []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := GlyphPath("flag"); got != plain {
		t.Fatalf("plain %q", got)
	}
}

type errStubType struct{}

func (errStubType) Error() string { return "down" }

var errStub error = errStubType{}

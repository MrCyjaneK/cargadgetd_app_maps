package nav

import (
	"strconv"
	"strings"
)

func parseFetch(out string) Trip {
	d := map[string]string{}
	for _, ln := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(ln, "\t")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		d[k] = variantInner(v)
	}
	if len(d) == 0 {
		return Trip{}
	}
	return Trip{
		OK:             true,
		HasRoute:       gBool(d, "hasRoute"),
		Running:        gBool(d, "running"),
		AlongRoute:     gBool(d, "alongRoute"),
		Direction:      gFloat(d, "direction"),
		DirectionValid: gBool(d, "directionValid"),
		Icon:           gStr(d, "icon"),
		ManDist:        gStr(d, "manDist"),
		ManTime:        gStr(d, "manTime"),
		Street:         gStr(d, "street"),
		Narrative:      gStr(d, "narrative"),
		DestDist:       gStr(d, "destDist"),
		DestTime:       gStr(d, "destTime"),
		DestEta:        gStr(d, "destEta"),
		Progress:       gInt(d, "progress"),
		RoundaboutExit: gInt(d, "roundaboutExit"),
	}
}

func variantInner(v string) string {
	i := strings.IndexByte(v, '<')
	j := strings.LastIndexByte(v, '>')
	if i < 0 || j <= i {
		return strings.TrimSpace(v)
	}
	return strings.TrimSpace(v[i+1 : j])
}

func gStr(d map[string]string, k string) string {
	v := strings.TrimSpace(d[k])
	if len(v) >= 2 && v[0] == '\'' && v[len(v)-1] == '\'' {
		inner := v[1 : len(v)-1]
		return strings.ReplaceAll(inner, `\'`, "'")
	}
	return ""
}

func gBool(d map[string]string, k string) bool {
	return strings.TrimSpace(d[k]) == "true"
}

func gInt(d map[string]string, k string) int {
	v := strings.TrimSpace(d[k])
	v = strings.TrimPrefix(v, "int32 ")
	v = strings.TrimPrefix(v, "int64 ")
	v = strings.TrimPrefix(v, "uint32 ")
	n, _ := strconv.Atoi(v)
	return n
}

func gFloat(d map[string]string, k string) float64 {
	v := strings.TrimSpace(d[k])
	f, _ := strconv.ParseFloat(v, 64)
	return f
}

// parseRoute reads a D-Bus array of doubles, latitude then longitude.
// A trailing unpaired value is dropped.
func parseRoute(out string) []Point {
	i := strings.IndexByte(out, '[')
	j := strings.LastIndexByte(out, ']')
	if i < 0 || j <= i {
		return nil
	}
	fields := strings.Split(out[i+1:j], ",")
	pts := make([]Point, 0, len(fields)/2)
	for n := 0; n+1 < len(fields); n += 2 {
		lat, err1 := strconv.ParseFloat(strings.TrimSpace(fields[n]), 64)
		lon, err2 := strconv.ParseFloat(strings.TrimSpace(fields[n+1]), 64)
		if err1 != nil || err2 != nil {
			continue
		}
		pts = append(pts, Point{Lat: lat, Lon: lon})
	}
	if len(pts) == 0 {
		return nil
	}
	return pts
}

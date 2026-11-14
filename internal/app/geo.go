package app

import (
	"math"
	"sort"
)

const (
	metresPerDeg = 111320.0
	sideOffsetM  = 8.0 // how far from the street centreline an interpolated house sits
)

// line is a polyline in local planar metres around its first point
// (equirectangular, fine at street scale).
type line struct {
	lat0, lon0, k float64
	pts           [][2]float64
	cum           []float64 // arc length at each vertex
}

func newLine(lonLat [][2]float64) *line {
	l := &line{lat0: lonLat[0][1], lon0: lonLat[0][0]}
	l.k = math.Cos(l.lat0 * math.Pi / 180)
	for i, p := range lonLat {
		x, y := l.toXY(p[1], p[0])
		l.pts = append(l.pts, [2]float64{x, y})
		d := 0.0
		if i > 0 {
			d = l.cum[i-1] + math.Hypot(x-l.pts[i-1][0], y-l.pts[i-1][1])
		}
		l.cum = append(l.cum, d)
	}
	return l
}

func (l *line) toXY(lat, lon float64) (x, y float64) {
	return (lon - l.lon0) * l.k * metresPerDeg, (lat - l.lat0) * metresPerDeg
}

// project returns the arc length of the point on the line nearest to (lat, lon).
func (l *line) project(lat, lon float64) float64 {
	px, py := l.toXY(lat, lon)
	best, bestD := 0.0, math.Inf(1)
	for i := 1; i < len(l.pts); i++ {
		ax, ay, bx, by := l.pts[i-1][0], l.pts[i-1][1], l.pts[i][0], l.pts[i][1]
		dx, dy := bx-ax, by-ay
		t := 0.0
		if n := dx*dx + dy*dy; n > 0 {
			t = math.Max(0, math.Min(1, ((px-ax)*dx+(py-ay)*dy)/n))
		}
		if d := math.Hypot(px-ax-t*dx, py-ay-t*dy); d < bestD {
			best, bestD = l.cum[i-1]+t*math.Hypot(dx, dy), d
		}
	}
	return best
}

// at returns the point at arc length s, moved sideM metres to the left of the
// direction of travel (negative: right).
func (l *line) at(s, sideM float64) (lat, lon float64) {
	i := sort.SearchFloat64s(l.cum, s)
	i = max(1, min(i, len(l.pts)-1))
	ax, ay, bx, by := l.pts[i-1][0], l.pts[i-1][1], l.pts[i][0], l.pts[i][1]
	seg := l.cum[i] - l.cum[i-1]
	t := 0.0
	if seg > 0 {
		t = math.Max(0, math.Min(1, (s-l.cum[i-1])/seg))
	}
	x, y := ax+t*(bx-ax), ay+t*(by-ay)
	if seg > 0 {
		x += -(by - ay) / seg * sideM
		y += (bx - ax) / seg * sideM
	}
	return l.lat0 + y/metresPerDeg, l.lon0 + x/(l.k*metresPerDeg)
}

type anchor struct {
	num int
	s   float64
}

// interpolateS finds the arc length for house number n by linear interpolation between
// the known numbers on either side, clamped at the ends.
func interpolateS(anchors []anchor, n int) float64 {
	sort.Slice(anchors, func(i, j int) bool { return anchors[i].num < anchors[j].num })
	if n <= anchors[0].num {
		return anchors[0].s
	}
	for i := 1; i < len(anchors); i++ {
		a, b := anchors[i-1], anchors[i]
		if n <= b.num {
			if b.num == a.num {
				return b.s
			}
			return a.s + (b.s-a.s)*float64(n-a.num)/float64(b.num-a.num)
		}
	}
	return anchors[len(anchors)-1].s
}

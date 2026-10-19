package radar

import (
	"hash/fnv"
	"math"
	"sort"
)

// Geometry of the 500×500 radar (design/Radar.dc.html): the center, the ring
// radii from the center out, and the blip size.
const (
	Center  = 250.0
	minGap  = 19.0 // between blip centers: two 8px blips and a little air
	edgePad = 11.0 // from the quadrant axes and the ring lines
)

// RingRadii are the ring boundaries: adopt is 0 to 60, hold 180 to 240.
var RingRadii = [5]float64{0, 60, 120, 180, 240}

// startAngle is where each quadrant begins, in degrees clockwise from east
// (SVG y points down): languages top right, techniques top left, tools
// bottom left, platforms bottom right.
var startAngle = [4]float64{-90, 180, 90, 0}

// Point is a blip's place on the radar, plus the radial direction for the
// "moved" trail.
type Point struct {
	X, Y   float64
	DX, DY float64 // unit vector from the center through the blip
}

// Layout places every blip inside its quadrant and ring. A blip's spot
// comes from a hash of its group id, so it stays put across renders and
// editions while it keeps its quadrant and ring; blips are placed in hash
// order and step aside from ones already placed, so none overlap.
func Layout(bs []Blip) map[string]Point {
	order := make([]int, len(bs))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return seed(bs[order[a]].Group) < seed(bs[order[b]].Group) })
	out := make(map[string]Point, len(bs))
	var placed []Point
	for _, i := range order {
		b := bs[i]
		q, r := QuadrantIndex(b.Quadrant), RingIndex(b.Ring)
		if q < 0 || r < 0 {
			continue
		}
		p := place(seed(b.Group), q, r, placed)
		placed = append(placed, p)
		out[b.Group] = p
	}
	return out
}

func seed(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}

// place finds a free spot in quadrant q, ring r: the hashed spot first, then
// the free spot of the segment nearest it, and in a crowded segment (adopt
// is small) the spot farthest from the others.
func place(h uint64, q, r int, placed []Point) Point {
	r0, r1 := RingRadii[r]+edgePad, RingRadii[r+1]-edgePad
	if r == 0 {
		r0 = 14 // the center is crowded by the axes
	}
	frac := func(bits uint64) float64 { return float64(bits%10007) / 10006 }
	fa, fr := frac(h), frac(h>>21)
	at := func(fa, fr float64) Point {
		rad := r0 + fr*(r1-r0)
		// Keep clear of the axes: the margin is an arc of edgePad at this radius.
		pad := edgePad / rad * 180 / math.Pi
		if pad > 30 {
			pad = 30
		}
		deg := startAngle[q] + pad + fa*(90-2*pad)
		a := deg * math.Pi / 180
		dx, dy := math.Cos(a), math.Sin(a)
		return Point{X: Center + rad*dx, Y: Center + rad*dy, DX: dx, DY: dy}
	}
	clearance := func(p Point) float64 {
		d := math.Inf(1)
		for _, o := range placed {
			d = math.Min(d, math.Hypot(p.X-o.X, p.Y-o.Y))
		}
		return d
	}
	best := at(fa, fr)
	if clearance(best) >= minGap {
		return best
	}
	// Else the spot of the segment with the most room around it, nearest
	// the hashed one among those with room enough.
	var roomy, widest Point
	roomyDist, most := math.Inf(1), -1.0
	const n = 36
	for i := 0; i <= n; i++ {
		for j := 0; j <= n; j++ {
			p := at(float64(i)/n, float64(j)/n)
			c := clearance(p)
			if c > most {
				widest, most = p, c
			}
			if d := math.Hypot(p.X-best.X, p.Y-best.Y); c >= minGap && d < roomyDist {
				roomy, roomyDist = p, d
			}
		}
	}
	if !math.IsInf(roomyDist, 1) {
		return roomy
	}
	return widest // a crowded segment: as far from the others as it gets
}

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var areaPlaces = map[string]bool{"suburb": true, "quarter": true, "neighbourhood": true, "locality": true}
var landmarkKeys = []string{"railway", "amenity", "tourism", "historic", "leisure", "place"}

// ImportGazetteer loads named areas and landmarks of the Settlement with the given ЕКАТТЕ from an
// Overpass-JSON extract (`out geom`/`out center`). A closed way tagged place=suburb/quarter/
// neighbourhood/locality is an area; any other named railway/amenity/tourism/historic/leisure/place
// element becomes a point (its node, centre or centroid). Re-running updates in place.
// shortcut: relations (multipolygon areas) are skipped; Operators draw those until ticket 18/49 lands.
func ImportGazetteer(ctx context.Context, dsn string, r io.Reader, ekatte string) error {
	var doc struct{ Elements []osmElement }
	if err := json.NewDecoder(r).Decode(&doc); err != nil {
		return fmt.Errorf("parse extract: %w", err)
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var sid int
	if err := tx.QueryRow(ctx, `SELECT id FROM settlements WHERE ekatte = $1`, ekatte).Scan(&sid); err != nil {
		return fmt.Errorf("settlement %s: %w", ekatte, err)
	}
	for _, e := range doc.Elements {
		name := e.Tags["name"]
		if name == "" || e.Tags["highway"] != "" || e.Tags["ref:ekatte"] != "" {
			continue
		}
		var kind string
		var pts [][2]float64
		for _, g := range e.Geometry {
			pts = append(pts, [2]float64{g.Lon, g.Lat})
		}
		switch {
		case areaPlaces[e.Tags["place"]] && len(pts) >= 4 && pts[0] == pts[len(pts)-1]:
			kind = "area"
		case hasAny(e.Tags, landmarkKeys):
			kind = "point"
			switch {
			case e.Type == "node":
				pts = [][2]float64{{e.Lon, e.Lat}}
			case e.Center != nil:
				pts = [][2]float64{{e.Center.Lon, e.Center.Lat}}
			case len(pts) > 0:
				var x, y float64
				for _, p := range pts {
					x, y = x+p[0], y+p[1]
				}
				pts = [][2]float64{{x / float64(len(pts)), y / float64(len(pts))}}
			default:
				continue
			}
		default:
			continue
		}
		geom, _ := json.Marshal(pts)
		if _, err := tx.Exec(ctx, `INSERT INTO gazetteer (settlement_id, name, search, kind, geom) VALUES ($1,$2,$3,$4,$5)
			ON CONFLICT (settlement_id, name) DO UPDATE SET search = EXCLUDED.search, kind = EXCLUDED.kind, geom = EXCLUDED.geom`,
			sid, name, areaKey(name), kind, geom); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func hasAny(tags map[string]string, keys []string) bool {
	for _, k := range keys {
		if tags[k] != "" {
			return true
		}
	}
	return false
}

// areaKey is the comparison form of an area or landmark name: normalized, area-type words dropped.
func areaKey(s string) string {
	var w []string
	for _, f := range strings.Fields(normalize(s)) {
		switch f {
		case "kv", "zhk", "m-st", "m-t", "k-s", "mestnost", "kvartal", "s", "gr", "na", "ot", "v", "i":
		default:
			w = append(w, f)
		}
	}
	return strings.Join(w, " ")
}

type gazEntry struct {
	key, kind string
	pts       [][2]float64
}

// resolver knows what one Settlement's names resolve to: its streets and its Gazetteer.
type resolver struct {
	pool    *pgxpool.Pool
	ctx     context.Context
	id      int
	name    string
	village bool
	streets map[string]bool // normalized street names
	entries []gazEntry      // longest key first
	geoms   map[string][][2]float64
}

// loadResolver returns nil if no Settlement has this name.
func loadResolver(ctx context.Context, pool *pgxpool.Pool, settlement string) (*resolver, error) {
	r := &resolver{pool: pool, ctx: ctx, streets: map[string]bool{}, geoms: map[string][][2]float64{}}
	var kind string
	if err := pool.QueryRow(ctx, `SELECT id, name, kind FROM settlements WHERE lower(name) = lower($1)`, strings.TrimSpace(settlement)).Scan(&r.id, &r.name, &kind); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	r.village = kind == "village"
	rows, err := pool.Query(ctx, `SELECT search FROM streets WHERE settlement_id = $1`, r.id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			rows.Close()
			return nil, err
		}
		r.streets[s] = true
	}
	rows.Close()
	rows, err = pool.Query(ctx, `SELECT search, kind, geom FROM gazetteer WHERE settlement_id = $1 ORDER BY length(search) DESC`, r.id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var e gazEntry
		var g []byte
		if err := rows.Scan(&e.key, &e.kind, &g); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(g, &e.pts); err != nil {
			return nil, err
		}
		r.entries = append(r.entries, e)
	}
	return r, rows.Err()
}

// streetPts returns the geometry of the street whose normalized name is search.
func (r *resolver) streetPts(search string) ([][2]float64, error) {
	if p, ok := r.geoms[search]; ok {
		return p, nil
	}
	var g []byte
	if err := r.pool.QueryRow(r.ctx, `SELECT geom FROM streets WHERE settlement_id = $1 AND search = $2`, r.id, search).Scan(&g); err != nil {
		return nil, err
	}
	var p [][2]float64
	if err := json.Unmarshal(g, &p); err != nil {
		return nil, err
	}
	r.geoms[search] = p
	return p, nil
}

// --- planar geometry in lon/lat, fine at city scale ---

func metres(a, b [2]float64) float64 {
	return math.Hypot((a[0]-b[0])*math.Cos(a[1]*math.Pi/180), a[1]-b[1]) * metresPerDeg
}

func contains(ring [][2]float64, p [2]float64) bool {
	in := false
	for i, j := 0, len(ring)-1; i < len(ring); j, i = i, i+1 {
		a, b := ring[i], ring[j]
		if (a[1] > p[1]) != (b[1] > p[1]) && p[0] < (b[0]-a[0])*(p[1]-a[1])/(b[1]-a[1])+a[0] {
			in = !in
		}
	}
	return in
}

// crossing returns where segments ab and cd meet.
func crossing(a, b, c, d [2]float64) ([2]float64, bool) {
	den := (b[0]-a[0])*(d[1]-c[1]) - (b[1]-a[1])*(d[0]-c[0])
	if den == 0 {
		return [2]float64{}, false
	}
	t := ((c[0]-a[0])*(d[1]-c[1]) - (c[1]-a[1])*(d[0]-c[0])) / den
	u := ((c[0]-a[0])*(b[1]-a[1]) - (c[1]-a[1])*(b[0]-a[0])) / den
	if t < 0 || t > 1 || u < 0 || u > 1 {
		return [2]float64{}, false
	}
	return [2]float64{a[0] + t*(b[0]-a[0]), a[1] + t*(b[1]-a[1])}, true
}

// hull is the convex hull (monotone chain).
func hull(ps [][2]float64) [][2]float64 {
	sort.Slice(ps, func(i, j int) bool { return ps[i][0] < ps[j][0] || ps[i][0] == ps[j][0] && ps[i][1] < ps[j][1] })
	cross := func(o, a, b [2]float64) float64 { return (a[0]-o[0])*(b[1]-o[1]) - (a[1]-o[1])*(b[0]-o[0]) }
	var h [][2]float64
	for pass := 0; pass < 2; pass++ {
		start := len(h)
		for _, p := range ps {
			for len(h) >= start+2 && cross(h[len(h)-2], h[len(h)-1], p) <= 0 {
				h = h[:len(h)-1]
			}
			h = append(h, p)
		}
		h = h[:len(h)-1]
		for i, j := 0, len(ps)-1; i < j; i, j = i+1, j-1 {
			ps[i], ps[j] = ps[j], ps[i]
		}
	}
	return h
}

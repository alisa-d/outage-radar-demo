package app

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type feature struct {
	Type       string         `json:"type"`
	Geometry   map[string]any `json:"geometry"`
	Properties map[string]any `json:"properties"`
}

// parsePlace splits "[част от ]ул. Name [numbers]" into the street name and the listed building numbers.
func parsePlace(place string) (part bool, name string, listed []string, ok bool) {
	m := streetPlace.FindStringSubmatch(strings.TrimSpace(place))
	if m == nil {
		return
	}
	toks := strings.Fields(m[2])
	words := []string{strings.Trim(toks[0], ",.")}
	i := 1
	for ; i < len(toks) && !numberToken.MatchString(strings.Trim(toks[i], ",.")); i++ {
		words = append(words, strings.Trim(toks[i], ",."))
	}
	for _, n := range strings.FieldsFunc(strings.Join(toks[i:], " "), func(r rune) bool { return r == ',' || r == ' ' }) {
		n = strings.TrimRight(n, ".")
		if !numberToken.MatchString(n) {
			break // the rest is prose ("електрозахранени от ТП 314")
		}
		listed = append(listed, n)
	}
	return m[1] != "", strings.Join(words, " "), listed, true
}

// outageArea resolves the street-listed places of an Outage to GeoJSON: the street line, or the
// listed building points. "част от" streets are Possibly affected. An empty result means Unlocated.
// shortcut: street places only (like matchPlace); ticket 08 adds the Gazetteer and prose areas.
func outageArea(ctx context.Context, pool *pgxpool.Pool, settlement string, places []string) []feature {
	out := []feature{}
	for _, pl := range places {
		part, name, listed, ok := parsePlace(pl)
		if !ok {
			continue
		}
		var id int
		var geom []byte
		if pool.QueryRow(ctx, `SELECT st.id, st.geom FROM streets st JOIN settlements s ON s.id = st.settlement_id
			WHERE lower(s.name) = lower($1) AND st.search = $2`, strings.TrimSpace(settlement), normalize(name)).Scan(&id, &geom) != nil {
			continue
		}
		conf := affected
		if part {
			conf = possibly
		}
		props := map[string]any{"confidence": conf, "name": name}
		if len(listed) == 0 {
			var line [][2]float64
			if json.Unmarshal(geom, &line) == nil {
				out = append(out, feature{"Feature", map[string]any{"type": "LineString", "coordinates": line}, props})
			}
			continue
		}
		var pts [][2]float64
		for _, n := range listed {
			if b, err := lookupNumber(ctx, pool, strconv.Itoa(id), n); err == nil {
				pts = append(pts, [2]float64{b.Lon, b.Lat})
			}
		}
		if len(pts) > 0 {
			out = append(out, feature{"Feature", map[string]any{"type": "MultiPoint", "coordinates": pts}, props})
		}
	}
	return out
}

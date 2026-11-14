package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var translit = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ж': "zh", 'з': "z", 'и': "i", 'й': "y",
	'к': "k", 'л': "l", 'м': "m", 'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u",
	'ф': "f", 'х': "h", 'ц': "ts", 'ч': "ch", 'ш': "sh", 'щ': "sht", 'ъ': "a", 'ь': "", 'ю': "yu", 'я': "ya",
}

// Street-type words dropped before matching, in Cyrillic and Latin.
var streetTypes = map[string]bool{
	"ул": true, "улица": true, "бул": true, "булевард": true, "пл": true, "площад": true,
	"ul": true, "ulitsa": true, "bul": true, "bulevard": true, "blvd": true, "pl": true, "ploshtad": true,
}

// normalize lowercases, transliterates Cyrillic to Latin and drops street-type words.
// shortcut: one transliteration system (streamlined), no ъ/щ spelling variants; add variants when users miss streets.
func normalize(s string) string {
	var words []string
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return r == ' ' || r == '.' || r == ',' }) {
		if streetTypes[w] {
			continue
		}
		var b strings.Builder
		for _, r := range w {
			if l, ok := translit[r]; ok {
				b.WriteString(l)
			} else {
				b.WriteRune(r)
			}
		}
		words = append(words, b.String())
	}
	return strings.Join(words, " ")
}

var errNotFound = errors.New("not found")

type building struct {
	Number      string  `json:"number"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	Approximate bool    `json:"approximate"`
}

func leadingInt(s string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimRightFunc(s, func(r rune) bool { return !unicode.IsDigit(r) }))
	return n, err == nil
}

// lookupNumber returns the registry position of a building, interpolating along the
// street when OSM lacks the number: odd numbers on the left of the street's direction,
// even on the right.
func lookupNumber(ctx context.Context, pool *pgxpool.Pool, streetID, number string) (building, error) {
	id, err := strconv.Atoi(streetID)
	num, ok := leadingInt(number)
	if err != nil || !ok {
		return building{}, errNotFound
	}
	number = strings.ToUpper(number)
	var geom []byte
	if err := pool.QueryRow(ctx, `SELECT geom FROM streets WHERE id = $1`, id).Scan(&geom); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return building{}, errNotFound
		}
		return building{}, err
	}
	rows, err := pool.Query(ctx, `SELECT number, lat, lon FROM house_numbers WHERE street_id = $1`, id)
	if err != nil {
		return building{}, err
	}
	defer rows.Close()
	var known []building
	for rows.Next() {
		var b building
		if err := rows.Scan(&b.Number, &b.Lat, &b.Lon); err != nil {
			return building{}, err
		}
		if b.Number == number {
			return b, nil
		}
		known = append(known, b)
	}
	var pts [][2]float64
	if err := json.Unmarshal(geom, &pts); err != nil {
		return building{}, err
	}
	ln := newLine(pts)
	var same, all []anchor
	for _, b := range known {
		if k, ok := leadingInt(b.Number); ok {
			a := anchor{k, ln.project(b.Lat, b.Lon)}
			all = append(all, a)
			if k%2 == num%2 {
				same = append(same, a)
			}
		}
	}
	if len(same) == 0 {
		same = all
	}
	if len(same) == 0 {
		return building{}, errNotFound
	}
	side := sideOffsetM
	if num%2 == 0 {
		side = -side
	}
	lat, lon := ln.at(interpolateS(same, num), side)
	return building{Number: number, Lat: lat, Lon: lon, Approximate: true}, nil
}

type latLon struct{ Lat, Lon float64 }

type osmElement struct {
	Type     string            `json:"type"`
	Lat      float64           `json:"lat"`
	Lon      float64           `json:"lon"`
	Tags     map[string]string `json:"tags"`
	Geometry []latLon          `json:"geometry"`
	Center   *latLon           `json:"center"`
}

// ImportOSM loads an Overpass-JSON extract (`out geom`/`out center`): places with
// ref:ekatte become Settlements of the first Region; streets and house numbers
// attach to the Settlement with the given ЕКАТТЕ. Re-running updates in place.
func ImportOSM(ctx context.Context, dsn string, r io.Reader, ekatte string) error {
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

	for _, e := range doc.Elements {
		if e.Tags["place"] != "" && e.Tags["ref:ekatte"] != "" && e.Tags["name"] != "" {
			// Demo has one Region; settlements join it.
			if _, err := tx.Exec(ctx, `INSERT INTO settlements (region_id, name, ekatte) VALUES ((SELECT min(id) FROM regions), $1, $2)
				ON CONFLICT (ekatte) DO UPDATE SET name = EXCLUDED.name`, e.Tags["name"], e.Tags["ref:ekatte"]); err != nil {
				return err
			}
		}
	}
	var sid int
	if err := tx.QueryRow(ctx, `SELECT id FROM settlements WHERE ekatte = $1`, ekatte).Scan(&sid); err != nil {
		return fmt.Errorf("settlement %s: %w", ekatte, err)
	}
	streetID := map[string]int{}
	for _, e := range doc.Elements {
		name := e.Tags["name"]
		if e.Type != "way" || e.Tags["highway"] == "" || name == "" || len(e.Geometry) < 2 {
			continue
		}
		pts := make([][2]float64, len(e.Geometry))
		for i, g := range e.Geometry {
			pts[i] = [2]float64{g.Lon, g.Lat}
		}
		geom, _ := json.Marshal(pts)
		var id int
		// shortcut: a street split into several OSM ways keeps only the last way's geometry; stitch ways when it matters.
		if err := tx.QueryRow(ctx, `INSERT INTO streets (settlement_id, name, search, geom) VALUES ($1, $2, $3, $4)
			ON CONFLICT (settlement_id, name) DO UPDATE SET geom = EXCLUDED.geom RETURNING id`,
			sid, name, normalize(name), geom).Scan(&id); err != nil {
			return err
		}
		streetID[name] = id
	}
	for _, e := range doc.Elements {
		id, ok := streetID[e.Tags["addr:street"]]
		num := e.Tags["addr:housenumber"]
		lat, lon := e.Lat, e.Lon
		if e.Center != nil {
			lat, lon = e.Center.Lat, e.Center.Lon
		}
		if !ok || num == "" || (lat == 0 && lon == 0) {
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO house_numbers (street_id, number, lat, lon) VALUES ($1, $2, $3, $4)
			ON CONFLICT (street_id, number) DO UPDATE SET lat = EXCLUDED.lat, lon = EXCLUDED.lon`,
			id, strings.ToUpper(num), lat, lon); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

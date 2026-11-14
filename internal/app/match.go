package app

import (
	"regexp"
	"strings"
)

const radiusM = 300 // a point landmark is Possibly affected within this distance

var (
	streetPlace = regexp.MustCompile(`^(част от )?(?:улица|булевард|площад|ул|бул|пл)(?:\.\s*|\s+)(.+)$`)
	blockPlace  = regexp.MustCompile(`(?i)^(?:в района )?(?:между|в границите на)\s+(.+)$`)
	numberToken = regexp.MustCompile(`^\d+[\p{L}]?$`)
	blockSplit  = regexp.MustCompile(`\s*,\s*|\s+и\s+`)
)

// target is the Address being checked.
type target struct {
	street, number string
	pt             [2]float64 // lon, lat
}

// matchPlace is Seam 3 for one place description of an Outage in the resolver's Settlement. placed
// says whether the place resolved to anything (a street, block or Gazetteer entry) at all; a nil
// target only asks that. Confidence rules (ADR 0003): "част от"/"в района на" cap an area at Possibly
// affected, a landmark is Possibly affected within radiusM, a block with an unresolved street is capped.
// shortcut: a street place is matched by name and listed numbers only (no geometry), a block is the hull
// of the street crossings (convex blocks only), relation-shaped areas are not imported.
func matchPlace(r *resolver, place string, a *target) (res string, placed bool, err error) {
	place = strings.TrimSpace(place)
	if m := streetPlace.FindStringSubmatch(place); m != nil {
		return matchStreet(m, a), r.streets[streetKey(m[2])], nil
	}
	if m := blockPlace.FindStringSubmatch(place); m != nil {
		return matchBlock(r, m[1], a)
	}
	key := " " + areaKey(place) + " "
	lower := strings.ToLower(place)
	capped := strings.Contains(lower, "част от") || strings.Contains(lower, "района на") || strings.Contains(lower, "около")
	for _, e := range r.entries {
		if !strings.Contains(key, " "+e.key+" ") {
			continue
		}
		switch {
		case a == nil:
			return notAffected, true, nil
		case e.kind == "point":
			if metres(e.pts[0], a.pt) <= radiusM {
				return possibly, true, nil
			}
		case contains(e.pts, a.pt) && capped:
			return possibly, true, nil
		case contains(e.pts, a.pt):
			return affected, true, nil
		}
		return notAffected, true, nil
	}
	return notAffected, false, nil
}

// streetKey is the registry comparison form of "Name [numbers]": the words before the first number.
func streetKey(rest string) string {
	var name []string
	for _, t := range strings.Fields(rest) {
		if numberToken.MatchString(strings.Trim(t, ",.")) {
			break
		}
		name = append(name, strings.Trim(t, ",."))
	}
	return normalize(strings.Join(name, " "))
}

func matchStreet(m []string, a *target) string {
	if a == nil || streetKey(m[2]) != normalize(a.street) {
		return notAffected
	}
	toks := strings.Fields(m[2])
	i := 1
	for ; i < len(toks) && !numberToken.MatchString(strings.Trim(toks[i], ",.")); i++ {
	}
	listed := strings.FieldsFunc(strings.Join(toks[i:], " "), func(r rune) bool { return r == ',' || r == ' ' })
	for _, n := range listed {
		n = strings.TrimRight(n, ".")
		if !numberToken.MatchString(n) {
			break // the rest is prose ("електрозахранени от ТП 314")
		}
		if strings.EqualFold(n, a.number) {
			return affected
		}
	}
	switch {
	case len(listed) > 0 && numberToken.MatchString(strings.TrimRight(listed[0], ".")):
		return notAffected // numbers are listed and this one is not among them
	case m[1] != "":
		return possibly
	}
	return affected
}

// matchBlock resolves "between streets A, B, C and D" to the polygon spanned by where they cross.
func matchBlock(r *resolver, list string, a *target) (string, bool, error) {
	var lines [][][2]float64
	missing := false
	for _, t := range blockSplit.Split(list, -1) {
		if k := normalize(t); r.streets[k] {
			pts, err := r.streetPts(k)
			if err != nil {
				return notAffected, false, err
			}
			lines = append(lines, pts)
		} else {
			missing = true
		}
	}
	var corners [][2]float64
	for i := range lines {
		for j := i + 1; j < len(lines); j++ {
			for x := 1; x < len(lines[i]); x++ {
				for y := 1; y < len(lines[j]); y++ {
					if c, ok := crossing(lines[i][x-1], lines[i][x], lines[j][y-1], lines[j][y]); ok {
						corners = append(corners, c)
					}
				}
			}
		}
	}
	if len(corners) < 3 {
		return notAffected, false, nil
	}
	switch {
	case a == nil || !contains(hull(corners), a.pt):
		return notAffected, true, nil
	case missing:
		return possibly, true, nil // unresolved names lower the confidence
	}
	return affected, true, nil
}

// matchOutage combines the places of one Outage into a Match Result for the target. unlocated is true
// for a city Outage where nothing resolved (ADR 0003). In a village the whole Settlement is Affected when
// the Notice names it (or no place) and Possibly affected when nothing resolved. r may be nil
// (unknown Settlement name): nothing is known, so nothing is claimed.
func matchOutage(r *resolver, settlement string, places []string, a *target) (res string, unlocated bool, err error) {
	if r == nil || !strings.EqualFold(strings.TrimSpace(settlement), r.name) {
		return notAffected, false, nil
	}
	if r.village {
		whole := len(places) == 0
		for _, p := range places {
			whole = whole || areaKey(p) == areaKey(r.name) || strings.Contains(strings.ToLower(p), "цял")
		}
		if whole {
			return affected, false, nil
		}
	}
	res, anyPlaced := notAffected, false
	for _, p := range places {
		m, placed, err := matchPlace(r, p, a)
		if err != nil {
			return "", false, err
		}
		anyPlaced = anyPlaced || placed
		if m == affected || m == possibly && res == notAffected {
			res = m
		}
	}
	switch {
	case anyPlaced:
		return res, false, nil
	case r.village:
		return possibly, false, nil
	}
	return notAffected, true, nil
}

package app_test

import (
	"sync/atomic"
	"testing"

	"github.com/alisa/outage-radar/internal/testkit"
)

func TestRegionOutagesCarryAffectedAreaGeometryAndFlagUnlocated(t *testing.T) {
	s := testkit.Start(t, "all", nil)
	var down atomic.Bool
	pre := "Поради профилактика ще бъде прекъснато електрозахранването в районите на:<br/><br/>гр. Варна - "
	serveFeed(s, &down, feed(
		row(1, pre+"ул. Дойран, ул. Брегалница 3, 24."),
		row(2, pre+"част от ул. Фердинанд."),
		row(3, pre+"в района на парк Приморски."),
	))
	importVarna(t, s)
	ingested(t, s)

	var body struct {
		Days []struct {
			Settlements []struct {
				Outages []struct {
					Places    []string `json:"places"`
					Unlocated bool     `json:"unlocated"`
					Area      []struct {
						Geometry struct {
							Type string `json:"type"`
						} `json:"geometry"`
						Properties struct {
							Confidence string `json:"confidence"`
						} `json:"properties"`
					} `json:"area"`
				} `json:"outages"`
			} `json:"settlements"`
		} `json:"days"`
	}
	if c := getJSON(t, s.URL+"/api/regions/1/outages", &body); c != 200 {
		t.Fatalf("status %d", c)
	}
	got := map[string]string{}
	for _, d := range body.Days {
		for _, st := range d.Settlements {
			for _, o := range st.Outages {
				v := ""
				if o.Unlocated {
					v = "unlocated"
				}
				for _, f := range o.Area {
					v += f.Geometry.Type + "/" + f.Properties.Confidence + " "
				}
				got[o.Places[0]] = v
			}
		}
	}
	if v := got["ул. Дойран"]; v != "LineString/affected MultiPoint/affected " {
		t.Errorf("whole street and numbers: %q", v)
	}
	if v := got["част от ул. Фердинанд"]; v != "LineString/possibly_affected " {
		t.Errorf("part of street: %q", v)
	}
	if v := got["в района на парк Приморски"]; v != "unlocated" {
		t.Errorf("unresolvable: %q", v)
	}
}

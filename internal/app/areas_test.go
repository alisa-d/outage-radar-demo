package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alisa/outage-radar/internal/app"
	"github.com/alisa/outage-radar/internal/testkit"
)

// Fixtures varna.areas.osm.json and village.osm.json are SYNTHETIC geometry (not extracted from OSM).

// waterNotice is one ВиК Варна Notice and what the stubbed Claude extracts from it.
type waterNotice struct {
	text, settlement string
	places           []string
}

// serveWater replays the notices as ВиК Варна's emergency list and stubs Claude to answer by Notice text.
func serveWater(s *testkit.System, ns []waterNotice) {
	s.StubProviders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path == "/bg/messages.html" && q.Get("type") == "breakdown" && q.Get("page") == "1" {
			for i := range ns {
				fmt.Fprintf(w, `<h2><a href="https://vikvarna.com/bg/messages/breakdown/%d.html">T</a></h2><div class="list-item-date">14.11.2026</div><div class="list-item-text">x<a href="y">more</a></div>`, i+1)
			}
			return
		}
		var id int
		if _, err := fmt.Sscanf(r.URL.Path, "/bg/messages/breakdown/%d.html", &id); err == nil && id >= 1 && id <= len(ns) {
			fmt.Fprintf(w, `<div class="view"><p>%s</p><div class="clear">`, ns[id-1].text)
			return
		}
		if r.URL.Path == "/bg/messages.html" {
			return // empty lists
		}
		http.NotFound(w, r)
	}))
	s.StubClaude(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var req struct{ Messages []struct{ Content string } }
		json.Unmarshal(b, &req)
		out := `{"outages":[]}`
		for _, n := range ns {
			if len(req.Messages) == 1 && req.Messages[0].Content == n.text {
				p, _ := json.Marshal(n.places)
				out = fmt.Sprintf(`{"outages":[{"date":"","start":"09:00","end":"17:00","end_approximate":false,"severity":"cut","settlement":%q,"places":%s}]}`, n.settlement, p)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"type":"message","role":"assistant","stop_reason":"tool_use","content":[{"type":"tool_use","id":"t1","name":"record_outages","input":`+out+`}]}`)
	}))
}

func importFile(t *testing.T, s *testkit.System, file, ekatte string, gazetteer bool) {
	t.Helper()
	f, err := os.Open("testdata/" + file)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if gazetteer {
		err = app.ImportGazetteer(context.Background(), s.DSN, f, ekatte)
	} else {
		err = app.ImportOSM(context.Background(), s.DSN, f, ekatte)
	}
	if err != nil {
		t.Fatalf("import %s: %v", file, err)
	}
}

// startWater imports the fixtures, serves the notices and waits until ВиК Варна is Healthy.
func startWater(t *testing.T, ns ...waterNotice) *testkit.System {
	t.Helper()
	s := testkit.Start(t, "all", nil)
	importFile(t, s, "varna.areas.osm.json", "10135", false)
	importFile(t, s, "varna.areas.osm.json", "10135", true)
	importFile(t, s, "village.osm.json", "99001", false)
	serveWater(s, ns)
	s.Clock.Advance(16 * time.Minute) // the first poll ran before the stub was set
	waitFor(t, func() bool {
		var rs []struct {
			Providers []struct {
				Name   string                 `json:"name"`
				Health struct{ State string } `json:"health"`
			} `json:"providers"`
		}
		getJSON(t, s.URL+"/api/regions", &rs)
		for _, p := range rs[0].Providers {
			if p.Name == "ВиК Варна" {
				return p.Health.State == "healthy"
			}
		}
		return false
	})
	return s
}

type areaCheck struct {
	Result    string        `json:"result"`
	Outages   []checkOutage `json:"outages"`
	Unlocated []checkOutage `json:"unlocated"`
}

func waterAt(t *testing.T, s *testkit.System, ekatte, streetQ, num string) areaCheck {
	t.Helper()
	id := streets(t, s, ekatte, streetQ)[0].ID
	var out struct {
		Utilities []struct {
			Utility string `json:"utility"`
			areaCheck
		} `json:"utilities"`
	}
	if c := getJSON(t, fmt.Sprintf("%s/api/check?street=%d&number=%s&lang=bg", s.URL, id, num), &out); c != 200 {
		t.Fatalf("check status %d", c)
	}
	for _, u := range out.Utilities {
		if u.Utility == "water" {
			return u.areaCheck
		}
	}
	t.Fatal("no water utility")
	return areaCheck{}
}

func wantResult(t *testing.T, got areaCheck, want, label string) {
	t.Helper()
	if got.Result != want {
		t.Errorf("%s: %s, want %s (%+v)", label, got.Result, want, got)
	}
}

func TestBlockBetweenStreetsIsAffectedInsideAndNotAffectedOutside(t *testing.T) {
	s := startWater(t, waterNotice{"Спиране на водата между бул. Васил Левски, ул. Беласица, ул. Тодор Влайков и бул. Сливница", "Варна",
		[]string{"между бул. Васил Левски, ул. Беласица, ул. Тодор Влайков и бул. Сливница"}})
	wantResult(t, waterAt(t, s, "10135", "belasitsa", "7"), "affected", "inside the block")
	wantResult(t, waterAt(t, s, "10135", "levski", "120"), "not_affected", "on Левски, east of the block")
}

func TestPartOfAQuarterIsPossiblyAffectedInsideAndNotAffectedOutside(t *testing.T) {
	s := startWater(t, waterNotice{"Авария в част от кв. Аспарухово", "Варна", []string{"част от кв. Аспарухово"}})
	wantResult(t, waterAt(t, s, "10135", "asparuhov", "1"), "possibly_affected", "inside the quarter")
	wantResult(t, waterAt(t, s, "10135", "belasitsa", "7"), "not_affected", "outside the quarter")
}

func TestLandmarkGivesPossiblyAffectedWithinTheRadius(t *testing.T) {
	s := startWater(t,
		waterNotice{"Авария в района на ЖП гара", "Варна", []string{"в района на ЖП гара"}},
		waterNotice{"Ремонт при Фестивален и Конгресен център", "Варна", []string{"Фестивален и Конгресен център"}})
	// Both Outages are in force at once, so each Address sees exactly the landmark near it.
	gara := waterAt(t, s, "10135", "gara", "1")
	wantResult(t, gara, "possibly_affected", "near the station")
	if len(gara.Outages) != 1 || !strings.Contains(gara.Outages[0].Notice, "ЖП гара") {
		t.Errorf("station Outages: %+v", gara.Outages)
	}
	fk := waterAt(t, s, "10135", "morska", "2")
	wantResult(t, fk, "possibly_affected", "near the congress centre")
	if len(fk.Outages) != 1 || !strings.Contains(fk.Outages[0].Notice, "Конгресен") {
		t.Errorf("centre Outages: %+v", fk.Outages)
	}
	wantResult(t, waterAt(t, s, "10135", "belasitsa", "7"), "not_affected", "far from both")
}

func TestVillageWideNoticeAffectsEveryAddressInTheVillage(t *testing.T) {
	s := startWater(t, waterNotice{"Без вода цялото с. Каменар", "Каменар", nil})
	wantResult(t, waterAt(t, s, "99001", "parva", "3"), "affected", "village address")
	wantResult(t, waterAt(t, s, "10135", "belasitsa", "7"), "not_affected", "other settlement")
}

func TestUnplaceableVillageNoticeIsPossiblyAffectedForTheWholeSettlement(t *testing.T) {
	s := startWater(t, waterNotice{"Авария по ТП 3 в Каменар", "Каменар", []string{"ТП 3"}})
	wantResult(t, waterAt(t, s, "99001", "parva", "3"), "possibly_affected", "village address")
}

func TestCityNoticeWithNothingResolvableIsUnlocatedAndOnlyMentioned(t *testing.T) {
	s := startWater(t, waterNotice{"Авария, засегнати клиенти от ТП 314", "Варна", []string{"ТП 314"}})
	got := waterAt(t, s, "10135", "belasitsa", "7")
	wantResult(t, got, "not_affected", "address in the city")
	if len(got.Unlocated) != 1 || !strings.Contains(got.Unlocated[0].Notice, "ТП 314") || len(got.Outages) != 0 {
		t.Errorf("check must mention the Unlocated Outage: %+v", got)
	}
	if v := waterAt(t, s, "99001", "parva", "3"); len(v.Unlocated) != 0 {
		t.Errorf("other settlement mentions it: %+v", v)
	}
	var b struct {
		Days []struct {
			Settlements []struct {
				Outages []struct {
					Unlocated bool `json:"unlocated"`
				} `json:"outages"`
			} `json:"settlements"`
		} `json:"days"`
	}
	getJSON(t, s.URL+"/api/regions/1/outages", &b)
	if len(b.Days) != 1 || len(b.Days[0].Settlements[0].Outages) != 1 || !b.Days[0].Settlements[0].Outages[0].Unlocated {
		t.Errorf("Region summary must list it as Unlocated: %+v", b)
	}
}

func TestImportGazetteerIsRepeatable(t *testing.T) {
	s := testkit.Start(t, "all", nil)
	importFile(t, s, "varna.areas.osm.json", "10135", true)
	importFile(t, s, "varna.areas.osm.json", "10135", true)
}

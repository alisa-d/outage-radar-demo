package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"testing"

	"github.com/alisa/outage-radar/internal/app"
	"github.com/alisa/outage-radar/internal/testkit"
)

func importVarna(t *testing.T, s *testkit.System) {
	t.Helper()
	f, err := os.Open("testdata/varna.osm.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := app.ImportOSM(context.Background(), s.DSN, f, "10135"); err != nil {
		t.Fatalf("import: %v", err)
	}
}

func getJSON(t *testing.T, u string, out any) int {
	t.Helper()
	resp, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatal(err)
		}
	}
	return resp.StatusCode
}

type street struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func streets(t *testing.T, s *testkit.System, ekatte, q string) []street {
	var out []street
	if c := getJSON(t, s.URL+"/api/settlements/"+ekatte+"/streets?q="+url.QueryEscape(q), &out); c != 200 {
		t.Fatalf("status %d", c)
	}
	return out
}

func TestStreetAutocompleteMatchesCyrillicLatinAndAbbreviations(t *testing.T) {
	s := testkit.Start(t, "all", nil)
	importVarna(t, s)
	for _, q := range []string{"ферди", "Ferdinand", "ул. Фердинанд", "ul. ferdi", "ulitsa Ferdinand"} {
		got := streets(t, s, "10135", q)
		if len(got) != 1 || got[0].Name != "улица Фердинанд" {
			t.Errorf("q=%q -> %+v", q, got)
		}
	}
	if got := streets(t, s, "10135", "bul. vladislav"); len(got) != 1 || got[0].Name != "бул. Владислав Варненчик" {
		t.Errorf("boulevard: %+v", got)
	}
	if got := streets(t, s, "10135", "zzz"); len(got) != 0 {
		t.Errorf("zzz -> %+v", got)
	}
}

type building struct {
	Number      string  `json:"number"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	Approximate bool    `json:"approximate"`
}

func number(t *testing.T, s *testkit.System, streetID int, n string) building {
	var b building
	if c := getJSON(t, s.URL+"/api/streets/"+strconv.Itoa(streetID)+"/numbers/"+n, &b); c != 200 {
		t.Fatalf("number %s: status %d", n, c)
	}
	return b
}

func TestMissingHouseNumbersAreInterpolatedOnOppositeSidesAndFlaggedApproximate(t *testing.T) {
	s := testkit.Start(t, "all", nil)
	importVarna(t, s)
	id := streets(t, s, "10135", "ferdinand")[0].ID

	if b := number(t, s, id, "5"); b.Approximate || b.Lon != 27.904 || b.Lat != 43.20008 {
		t.Errorf("known number 5 = %+v", b)
	}
	odd, even := number(t, s, id, "7"), number(t, s, id, "4")
	if !odd.Approximate || !even.Approximate {
		t.Errorf("interpolated numbers must be approximate: %+v %+v", odd, even)
	}
	if odd.Lon < 27.904 || odd.Lon > 27.908 {
		t.Errorf("7 should lie between 5 and 9, got lon %v", odd.Lon)
	}
	if even.Lon < 27.9 || even.Lon > 27.908 {
		t.Errorf("4 should lie between 2 and 8, got lon %v", even.Lon)
	}
	if !(odd.Lat > 43.2 && even.Lat < 43.2) {
		t.Errorf("odd and even on opposite sides of the street (lat 43.2): odd %v even %v", odd.Lat, even.Lat)
	}
	var none building
	if c := getJSON(t, s.URL+"/api/streets/"+strconv.Itoa(id)+"/numbers/abc", &none); c != 404 {
		t.Errorf("non-numeric status %d", c)
	}
}

func TestImportIsRepeatableAndOtherSettlementsHaveNoStreets(t *testing.T) {
	s := testkit.Start(t, "all", nil)
	importVarna(t, s)
	importVarna(t, s)
	if got := streets(t, s, "10135", "ferdinand"); len(got) != 1 {
		t.Errorf("after re-import: %+v", got)
	}
	var regions []struct {
		Settlements []struct {
			Ekatte  string `json:"ekatte"`
			Streets bool   `json:"streets"`
		}
	}
	getJSON(t, s.URL+"/api/regions", &regions)
	got := map[string]bool{}
	for _, st := range regions[0].Settlements {
		got[st.Ekatte] = st.Streets
	}
	if len(got) != 2 || !got["10135"] || got["20482"] {
		t.Errorf("settlements streets flag = %v", got)
	}
}

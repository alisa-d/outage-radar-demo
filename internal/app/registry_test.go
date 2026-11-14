package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
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

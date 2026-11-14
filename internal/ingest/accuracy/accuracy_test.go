package accuracy_test

import (
	"testing"
	"time"

	"github.com/alisa/outage-radar/internal/ingest/accuracy"
	"github.com/alisa/outage-radar/internal/ingest/erpsever"
)

func TestCompareScoresPerField(t *testing.T) {
	t0 := time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)
	want := []accuracy.Outage{{Start: t0, End: t0.Add(3 * time.Hour), Severity: "cut", Kind: "planned", Settlement: "Варна", Places: []string{"ул. А", "ул. Б"}}}
	got := []accuracy.Outage{{Start: t0, End: t0.Add(4 * time.Hour), Severity: "cut", Kind: "planned", Settlement: "Варна", Places: []string{"ул.  б", "ул. А"}}}
	s := accuracy.Compare(want, got)
	if s["start"] != [2]int{1, 1} || s["end"] != [2]int{0, 1} || s["places"] != [2]int{1, 1} || s["outages"] != [2]int{1, 1} {
		t.Fatalf("unexpected score %v", s)
	}
	if m := accuracy.Compare(want, nil); m["outages"] != [2]int{0, 1} || m["start"] != [2]int{0, 1} {
		t.Fatalf("missing outage must count wrong: %v", m)
	}
}

func TestSetIsBigEnoughAndCoversBothProviders(t *testing.T) {
	cases, err := accuracy.Load("testdata")
	if err != nil {
		t.Fatal(err)
	}
	n := map[string]int{}
	for _, c := range cases {
		n[c.Provider]++
	}
	if len(cases) < 40 || n["ЕРП Север"] == 0 || n["ВиК Варна"] == 0 {
		t.Fatalf("need >=40 Notices from both Providers, have %v", n)
	}
}

// CI gate: the deterministic parser must reproduce every hand label, except
// cases that record a known_gap (those must still differ, so a fix removes the note).
func TestErpSeverParserMatchesLabels(t *testing.T) {
	cases, err := accuracy.Load("testdata")
	if err != nil {
		t.Fatal(err)
	}
	p := erpsever.New("")
	for _, c := range cases {
		if c.Provider != "ЕРП Север" {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			got, err := p.Extract(c.IngestNotice())
			if err != nil {
				t.Fatal(err)
			}
			perfect := accuracy.Compare(c.Expected, accuracy.FromIngest(got)).Perfect()
			switch {
			case c.KnownGap == "" && !perfect:
				t.Fatalf("parser differs from label:\n got %+v\nwant %+v", got, c.Expected)
			case c.KnownGap != "" && perfect:
				t.Fatalf("parser now matches; remove known_gap %q", c.KnownGap)
			}
		})
	}
}

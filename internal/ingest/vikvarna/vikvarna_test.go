package vikvarna_test

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alisa/outage-radar/internal/testkit"
)

type outage struct {
	Provider       string   `json:"provider"`
	Utility        string   `json:"utility"`
	Kind           string   `json:"kind"`
	Severity       string   `json:"severity"`
	Start          string   `json:"start"`
	End            string   `json:"end"`
	EndApproximate bool     `json:"end_approximate"`
	Places         []string `json:"places"`
	Notice         string   `json:"notice"`
}

// Fixtures were captured from https://vikvarna.com/bg/messages.html?region_id=15&sub_region_id=0&type=<type>&page=<n>
// and the Notices' own pages on 2026-11-14 (region 15 = Район Варна), unmodified.
type providers struct {
	listHits atomic.Int32
	page3    atomic.Bool
	detail   map[string]string // Notice id -> id whose captured page to serve instead
}

func (p *providers) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	serve := func(name string) {
		b, err := os.ReadFile("testdata/" + name + ".html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Write(b)
	}
	q := r.URL.Query()
	if r.URL.Path == "/bg/messages.html" && q.Get("region_id") == "15" {
		p.listHits.Add(1)
		if q.Get("page") == "3" {
			p.page3.Store(true)
		}
		serve(q.Get("type") + "_p" + q.Get("page"))
		return
	}
	if rest, ok := strings.CutPrefix(r.URL.Path, "/bg/messages/"); ok {
		if parts := strings.Split(strings.TrimSuffix(rest, ".html"), "/"); len(parts) == 2 {
			id := parts[1]
			if o, ok := p.detail[id]; ok {
				id = o
			}
			serve("detail_" + parts[0] + "_" + id)
			return
		}
	}
	http.NotFound(w, r)
}

// claude is the stubbed Claude API: tool-use replies keyed by Notice text.
type claude struct {
	mu       sync.Mutex
	requests []string // the user message of each request
	bodies   []string
	fail     func(text string) string // "", "500" or "invalid"
}

func reply(text string) string {
	switch {
	case strings.Contains(text, "13.30 до около 19.00"):
		return `{"outages":[{"date":"","start":"13:30","end":"19:00","end_approximate":true,"severity":"cut","settlement":"Варна","places":["част от живущите около бул. '1 ви Май'","част от м-т Вилите на кв. Аспарухово"]}]}`
	case strings.Contains(text, "Розова Долина"):
		return `{"outages":[{"date":"","start":"11:30","end":"15:00","end_approximate":true,"severity":"cut","settlement":"Варна","places":["улица Розова Долина","улица Перикоп"]}]}`
	case strings.Contains(text, "на 25.09."):
		// The real Notice names a date in September; the stub names one inside the test's window.
		return `{"outages":[{"date":"15.11","start":"09:00","end":"19:00","end_approximate":true,"severity":"cut","settlement":"Варна","places":["ж.к. Владиславово"]}]}`
	}
	return `{"outages":[]}`
}

func (c *claude) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	var req struct {
		Messages []struct{ Content string } `json:"messages"`
	}
	json.Unmarshal(b, &req)
	text := ""
	if len(req.Messages) == 1 {
		text = req.Messages[0].Content
	}
	c.mu.Lock()
	c.requests = append(c.requests, text)
	c.bodies = append(c.bodies, string(b))
	fail := c.fail
	c.mu.Unlock()
	out := reply(text)
	if fail != nil {
		switch fail(text) {
		case "500":
			http.Error(w, "overloaded", http.StatusInternalServerError)
			return
		case "invalid":
			out = `{"outages":[{"date":"","start":"13:30","end":"19:00","end_approximate":false,"severity":"explosion","settlement":"Варна","places":[]}]}`
		}
	}
	w.Header().Set("Content-Type", "application/json")
	io.WriteString(w, `{"type":"message","role":"assistant","stop_reason":"tool_use","content":[{"type":"tool_use","id":"t1","name":"record_outages","input":`+out+`}]}`)
}

func start(t *testing.T, p *providers, c *claude) *testkit.System {
	// The newest captured Notices are from 2026-11-13, so the clock starts that day.
	s := testkit.StartAt(t, "all", nil, time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC))
	s.StubProviders(p)
	s.StubClaude(c)
	return s
}

func outages(t *testing.T, s *testkit.System, want int) []outage {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		var all []outage
		if resp, err := http.Get(s.URL + "/api/regions/1/outages?lang=bg"); err == nil {
			var body struct {
				Days []struct {
					Settlements []struct {
						Outages []outage `json:"outages"`
					} `json:"settlements"`
				} `json:"days"`
			}
			json.NewDecoder(resp.Body).Decode(&body)
			resp.Body.Close()
			for _, d := range body.Days {
				for _, st := range d.Settlements {
					all = append(all, st.Outages...)
				}
			}
		}
		if len(all) >= want {
			return all
		}
		if time.Now().After(deadline) {
			t.Fatalf("want %d outages after 15s, got %+v", want, all)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func find(t *testing.T, got []outage, textPart string) outage {
	t.Helper()
	for _, o := range got {
		if strings.Contains(o.Notice, textPart) {
			return o
		}
	}
	t.Fatalf("no outage for a Notice containing %q in %+v", textPart, got)
	return outage{}
}

func metrics(t *testing.T, s *testkit.System) string {
	resp, err := http.Get(s.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func TestCapturedPagesBecomeWaterOutagesWithKindFromTheList(t *testing.T) {
	p, c := &providers{}, &claude{}
	s := start(t, p, c)
	got := outages(t, s, 3)

	e := find(t, got, "13.30 до около 19.00")
	if e.Provider != "ВиК Варна" || e.Utility != "water" || e.Kind != "emergency" || e.Severity != "cut" {
		t.Fatalf("emergency = %+v", e)
	}
	// The Notice has no date, so its publication date (2026-11-13) applies; "около" flags the end approximate.
	if e.Start != "2026-11-13T13:30:00+02:00" || e.End != "2026-11-13T19:00:00+02:00" || !e.EndApproximate {
		t.Fatalf("emergency window = %+v", e)
	}
	if len(e.Places) != 2 {
		t.Fatalf("emergency places = %+v", e.Places)
	}
	if r := find(t, got, "на 25.09."); r.Kind != "planned" {
		t.Fatalf("repair-list Notice kind = %q", r.Kind)
	}

	// Claude receives each Notice's text alone, as one plain message.
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.requests) == 0 {
		t.Fatal("no extraction requests")
	}
	for i, text := range c.requests {
		if text == "" || strings.Contains(text, "@") || len(text) > 400 {
			t.Fatalf("unexpected extraction request: %s", c.bodies[i])
		}
	}
}

func TestIdenticalNoticeTextIsNeverSentToTheLLMTwice(t *testing.T) {
	// Notice 17819 is served with the page of 17818: two Notices, one text.
	p, c := &providers{detail: map[string]string{"17819": "17818"}}, &claude{}
	s := start(t, p, c)
	outages(t, s, 3)
	first := p.listHits.Load()
	s.Clock.Advance(16 * time.Minute) // a second poll
	for deadline := time.Now().Add(10 * time.Second); p.listHits.Load() < 2*first && time.Now().Before(deadline); {
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(500 * time.Millisecond)
	c.mu.Lock()
	defer c.mu.Unlock()
	seen := map[string]bool{}
	for _, r := range c.requests {
		if seen[r] {
			t.Fatalf("text sent twice: %q", r)
		}
		seen[r] = true
	}
	if len(c.requests) != 13 { // 14 in-window Notices, one duplicate text
		t.Fatalf("%d extraction requests", len(c.requests))
	}
}

func TestLLMFailureLeavesNoticeForRetryAndIsCounted(t *testing.T) {
	p, c := &providers{}, &claude{}
	var recovered atomic.Bool
	c.fail = func(text string) string {
		switch {
		case recovered.Load():
			return ""
		case strings.Contains(text, "13.30 до около 19.00"):
			return "500"
		case strings.Contains(text, "Розова Долина"):
			return "invalid"
		}
		return ""
	}
	s := start(t, p, c)
	got := outages(t, s, 1) // the repair Notice
	time.Sleep(time.Second)
	got = outages(t, s, 1)
	for _, o := range got {
		if strings.Contains(o.Notice, "13.30 до около 19.00") || strings.Contains(o.Notice, "Розова Долина") {
			t.Fatalf("outage invented for a failed extraction: %+v", o)
		}
	}
	if m := metrics(t, s); !regexp.MustCompile(`outage_radar_extraction_failures_total\{[^}]*provider="ВиК Варна"\} 2`).MatchString(m) {
		t.Fatalf("metrics:\n%s", m)
	}

	recovered.Store(true)
	s.Clock.Advance(16 * time.Minute)
	got = outages(t, s, 3)
	find(t, got, "13.30 до около 19.00")
	find(t, got, "Розова Долина")
}

func TestPagingStopsAtNoticesOlderThanTheWindow(t *testing.T) {
	p, c := &providers{}, &claude{}
	s := start(t, p, c)
	outages(t, s, 3)
	if p.page3.Load() {
		t.Fatal("requested page 3 although page 2 already reached older Notices")
	}
}

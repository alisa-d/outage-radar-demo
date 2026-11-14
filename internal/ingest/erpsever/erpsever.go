// Package erpsever is the ЕРП Север adapter: the oblast JSON endpoint plus a
// deterministic parser for its regular "гр. Варна - ул. X 1, 3, 5" format.
package erpsever

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
	_ "time/tzdata" // Europe/Sofia must resolve in a bare container

	"github.com/alisa/outage-radar/internal/ingest"
)

const varnaAreaID = "1" // the endpoint's id for Varna oblast

var sofia = func() *time.Location {
	l, err := time.LoadLocation("Europe/Sofia")
	if err != nil {
		panic(err)
	}
	return l
}()

type Adapter struct{ baseURL string }

func New(baseURL string) *Adapter { return &Adapter{strings.TrimRight(baseURL, "/")} }

func (*Adapter) Name() string            { return "ЕРП Север" }
func (*Adapter) Interval() time.Duration { return 15 * time.Minute }

type row struct {
	ID    json.Number `json:"id"`
	From  string      `json:"from"`
	To    string      `json:"to"`
	Hours struct {
		String string     `json:"string"`
		Array  [][]string `json:"array"`
	} `json:"hours"`
	PublishedAt string `json:"published_at"`
}

func (a *Adapter) Fetch(ctx context.Context, c *http.Client) ([]ingest.Notice, error) {
	var out []ingest.Notice
	for _, typ := range []string{"for_next_48_hours", "all_active"} {
		// shortcut: only offset 0; the endpoint returned every row (73) in one page, add paging if it starts to truncate.
		url := fmt.Sprintf("%s/bg/profil/xhr/?method=get_interruptions&region_id=%s&type=%s&offset=0", a.baseURL, varnaAreaID, typ)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		resp, err := c.Do(req)
		if err != nil {
			return nil, err
		}
		var areas []map[string]json.RawMessage
		err = json.NewDecoder(resp.Body).Decode(&areas)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("%s: status %d", typ, resp.StatusCode)
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w: %v", typ, ingest.ErrFormat, err)
		}
		for _, area := range areas {
			var id string
			json.Unmarshal(area["area_id"], &id)
			if id != varnaAreaID {
				continue
			}
			var rows []json.RawMessage
			if err := json.Unmarshal(area["total_location_result_rows_"+typ], &rows); err != nil {
				return nil, fmt.Errorf("%s rows: %w", typ, err)
			}
			for _, raw := range rows {
				var r row
				var m struct {
					Main string `json:"main"`
				}
				if err := json.Unmarshal(raw, &r); err != nil {
					return nil, fmt.Errorf("%s row: %w", typ, err)
				}
				json.Unmarshal(raw, &m)
				pub, _ := time.ParseInLocation("02.01.2006 15:04:05", r.PublishedAt, sofia)
				out = append(out, ingest.Notice{ExternalID: r.ID.String(), Published: pub, Text: plain(m.Main), Fields: raw})
			}
		}
	}
	return out, nil
}

var (
	br  = regexp.MustCompile(`<br\s*/?>`)
	tag = regexp.MustCompile(`<[^>]*>`)
	ws  = regexp.MustCompile(`[ \t\r\f]+`)
)

// plain turns the Provider's HTML fragment into text, keeping its words.
func plain(h string) string {
	lines := strings.Split(tag.ReplaceAllString(br.ReplaceAllString(h, "\n"), ""), "\n")
	var out []string
	for _, l := range lines {
		if l = strings.TrimSpace(ws.ReplaceAllString(l, " ")); l != "" {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

var (
	marker   = regexp.MustCompile(`(?:^|[\s,;])(?:гр|с)\.\s*(\p{L}{2,}(?:[ -]\p{L}{2,})*)`)
	placeKey = regexp.MustCompile(`(?:^|[;,]\s*)((?:част от |в района на )?(?:ул\.|бул\.|кв\.|м-ст|пл\.|ж\.к\.))`)
)

// Extract: one Outage per Settlement, per day of the window, per time range.
func (*Adapter) Extract(n ingest.Notice) ([]ingest.Outage, error) {
	var r row
	if err := json.Unmarshal(n.Fields, &r); err != nil {
		return nil, err
	}
	from, err1 := time.ParseInLocation("02.01.2006", r.From, sofia)
	to, err2 := time.ParseInLocation("02.01.2006", r.To, sofia)
	if err1 != nil || err2 != nil || len(r.Hours.Array) == 0 {
		return nil, fmt.Errorf("unreadable period %q-%q %q", r.From, r.To, r.Hours.String)
	}
	lower := strings.ToLower(n.Text)
	var severity string
	switch {
	case strings.Contains(lower, "прекъснато"):
		severity = "cut"
	case strings.Contains(lower, "смущения"):
		severity = "disturbance"
	default:
		return nil, fmt.Errorf("neither cut nor disturbance wording")
	}
	// shortcut: "около" is looked for in the hours text only; no captured Notice has an approximate end yet.
	approx := strings.Contains(strings.ToLower(r.Hours.String), "около")

	type segment struct {
		settlement string
		places     []string
	}
	body := strings.Join(strings.Fields(n.Text), " ")
	if i := strings.Index(body, "районите на:"); i >= 0 {
		body = body[i+len("районите на:"):]
	}
	var segs []segment
	ms := marker.FindAllStringSubmatchIndex(body, -1)
	for i, m := range ms {
		end := len(body)
		if i+1 < len(ms) {
			end = ms[i+1][0]
		}
		segs = append(segs, segment{body[m[2]:m[3]], places(body[m[1]:end])})
	}
	if len(segs) == 0 {
		segs = []segment{{"", places(body)}}
	}

	var out []ingest.Outage
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		for _, h := range r.Hours.Array {
			if len(h) != 2 {
				return nil, fmt.Errorf("bad hours %v", h)
			}
			s, e := at(d, h[0]), at(d, h[1])
			if s.IsZero() || e.IsZero() {
				return nil, fmt.Errorf("bad hours %v", h)
			}
			for _, sg := range segs {
				out = append(out, ingest.Outage{Start: s, End: e, EndApproximate: approx, Severity: severity, Kind: "planned", Settlement: sg.settlement, Places: sg.places})
			}
		}
	}
	return out, nil
}

func at(day time.Time, hm string) time.Time {
	t, err := time.Parse("15:04", hm)
	if err != nil {
		return time.Time{}
	}
	return time.Date(day.Year(), day.Month(), day.Day(), t.Hour(), t.Minute(), 0, 0, sofia)
}

// places splits "ул. A 1, 3, ул. B 5" at each street/quarter/area keyword.
func places(rest string) []string {
	rest = strings.Trim(rest, " -–—:,.;")
	var cuts []int
	for _, m := range placeKey.FindAllStringSubmatchIndex(rest, -1) {
		cuts = append(cuts, m[2])
	}
	if len(cuts) == 0 || cuts[0] != 0 {
		cuts = append([]int{0}, cuts...)
	}
	cuts = append(cuts, len(rest))
	var out []string
	for i := 0; i+1 < len(cuts); i++ {
		if p := strings.Trim(rest[cuts[i]:cuts[i+1]], " ,;."); p != "" && p != "и" {
			out = append(out, p)
		}
	}
	return out
}

// Package vikvarna is the ВиК Варна adapter: the site's HTML lists of emergency
// (breakdown) and planned (repair) Notices for Район Варна. Prose goes to an
// Extractor (the shared LLM extractor); the list a Notice came from sets its Kind.
package vikvarna

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
	_ "time/tzdata" // Europe/Sofia must resolve in a bare container

	"github.com/alisa/outage-radar/internal/ingest"
)

const (
	varnaRegionID = "15" // the site's id for Район Варна
	maxPages      = 10
	window        = 48 * time.Hour // Notices older than the newest minus this are not fetched
)

var sofia = func() *time.Location {
	l, err := time.LoadLocation("Europe/Sofia")
	if err != nil {
		panic(err)
	}
	return l
}()

// lists maps the site's list type to the Outage Kind it carries.
var lists = []struct{ typ, kind string }{{"breakdown", "emergency"}, {"repair", "planned"}}

type Adapter struct {
	baseURL string
	ex      ingest.Extractor
	// shortcut: detail pages cached in memory per id+list snippet, lost on restart (one refetch); no expiry.
	details map[string]string
}

func New(baseURL string, ex ingest.Extractor) *Adapter {
	return &Adapter{strings.TrimRight(baseURL, "/"), ex, map[string]string{}}
}

func (*Adapter) Name() string            { return "ВиК Варна" }
func (*Adapter) Interval() time.Duration { return 15 * time.Minute }

var (
	item    = regexp.MustCompile(`(?s)<h2><a href="[^"]*/messages/\w+/(\d+)\.html[^"]*">.*?list-item-date">\s*(\d\d\.\d\d\.\d{4})[^<]*</div>\s*<div class="list-item-text">(.*?)<a `)
	view    = regexp.MustCompile(`(?s)<div class="view">(.*?)<div class="clear">`)
	br      = regexp.MustCompile(`</p>|` + ingest.Br.String())
	newline = regexp.MustCompile(`\s*\n\s*`)
)

func get(ctx context.Context, c *http.Client, url string) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || err != nil {
		return "", fmt.Errorf("%s: status %d, read: %v", url, resp.StatusCode, err)
	}
	return string(b), nil
}

func (a *Adapter) Fetch(ctx context.Context, c *http.Client) ([]ingest.Notice, error) {
	var out []ingest.Notice
	for _, l := range lists {
		var cutoff time.Time
	pages:
		for page := 1; page <= maxPages; page++ {
			h, err := get(ctx, c, fmt.Sprintf("%s/bg/messages.html?region_id=%s&sub_region_id=0&type=%s&page=%d", a.baseURL, varnaRegionID, l.typ, page))
			if err != nil {
				return nil, err
			}
			items := item.FindAllStringSubmatch(h, -1)
			if len(items) == 0 {
				break
			}
			for _, m := range items {
				pub, err := time.ParseInLocation("02.01.2006", m[2], sofia)
				if err != nil {
					return nil, err
				}
				if cutoff.IsZero() { // lists are newest first
					cutoff = pub.Add(-window)
				}
				if pub.Before(cutoff) {
					break pages // only as far as needed
				}
				id, snippet := m[1], strings.TrimSpace(ingest.Tag.ReplaceAllString(m[3], ""))
				key := l.typ + id + "|" + snippet
				text, ok := a.details[key]
				if !ok {
					// The list text is cut short; the Notice's own page has all of it.
					d, err := get(ctx, c, fmt.Sprintf("%s/bg/messages/%s/%s.html", a.baseURL, l.typ, id))
					if err != nil {
						return nil, err
					}
					v := view.FindStringSubmatch(d)
					if v == nil {
						return nil, fmt.Errorf("notice %s/%s: no text found", l.typ, id)
					}
					text = plain(v[1])
					a.details[key] = text
				}
				fields, _ := json.Marshal(map[string]string{"kind": l.kind})
				out = append(out, ingest.Notice{ExternalID: l.typ + "/" + id, Published: pub, Text: text, Fields: fields})
			}
		}
	}
	return out, nil
}

func plain(h string) string {
	s := html.UnescapeString(ingest.StripTags(h, br))
	s = newline.ReplaceAllString(ingest.Spaces.ReplaceAllString(strings.ReplaceAll(s, " ", " "), " "), "\n")
	return strings.TrimSpace(s)
}

// Extract sends the prose to the extractor; the Kind comes from the list.
func (a *Adapter) Extract(n ingest.Notice) ([]ingest.Outage, error) {
	var f struct{ Kind string }
	if err := json.Unmarshal(n.Fields, &f); err != nil {
		return nil, err
	}
	outs, err := a.ex.Extract(n)
	for i := range outs {
		outs[i].Kind = f.Kind
	}
	return outs, err
}

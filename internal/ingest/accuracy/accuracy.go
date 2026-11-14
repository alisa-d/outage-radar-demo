// Package accuracy is Seam 2: a labelled set of real Notices with the Outages a
// careful human read out of them, and the scoring of an extractor against it.
// A case is one JSON file under testdata/; adding a Notice is adding a file.
package accuracy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alisa/outage-radar/internal/ingest"
)

// Fields are scored in this order.
var Fields = []string{"outages", "start", "end", "end_approximate", "severity", "kind", "settlement", "places"}

type Outage struct {
	Start          time.Time `json:"start"`
	End            time.Time `json:"end"`
	EndApproximate bool      `json:"end_approximate"`
	Severity       string    `json:"severity"`
	Kind           string    `json:"kind"`
	Settlement     string    `json:"settlement"`
	Places         []string  `json:"places"`
}

type Case struct {
	Name     string `json:"-"`
	Provider string `json:"provider"` // ЕРП Север has a deterministic parser; ВиК Варна is LLM only
	Notice   struct {
		ExternalID string          `json:"external_id"`
		Published  time.Time       `json:"published"`
		Text       string          `json:"text"`
		Fields     json.RawMessage `json:"fields"`
	} `json:"notice"`
	Expected []Outage `json:"expected"`
	// KnownGap, when set, says why the deterministic parser is expected to differ from Expected today.
	KnownGap string `json:"known_gap,omitempty"`
}

func (c Case) IngestNotice() ingest.Notice {
	return ingest.Notice{ExternalID: c.Notice.ExternalID, Published: c.Notice.Published, Text: c.Notice.Text, Fields: c.Notice.Fields}
}

func Load(dir string) ([]Case, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	var out []Case
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var c Case
		d := json.NewDecoder(strings.NewReader(string(b)))
		d.DisallowUnknownFields()
		if err := d.Decode(&c); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		c.Name = strings.TrimSuffix(filepath.Base(f), ".json")
		out = append(out, c)
	}
	return out, nil
}

// Score counts correct and total per field.
type Score map[string][2]int

func (s Score) add(field string, ok bool) {
	v := s[field]
	if ok {
		v[0]++
	}
	v[1]++
	s[field] = v
}

// Add merges o into s.
func (s Score) Add(o Score) {
	for k, v := range o {
		t := s[k]
		s[k] = [2]int{t[0] + v[0], t[1] + v[1]}
	}
}

func (s Score) Perfect() bool {
	for _, v := range s {
		if v[0] != v[1] {
			return false
		}
	}
	return true
}

func (s Score) String() string {
	var b strings.Builder
	for _, f := range Fields {
		if v, ok := s[f]; ok {
			fmt.Fprintf(&b, "%-16s %3d/%-3d %5.1f%%\n", f, v[0], v[1], 100*float64(v[0])/float64(v[1]))
		}
	}
	return b.String()
}

func key(o Outage) string {
	p := ""
	if len(o.Places) > 0 {
		p = o.Places[0]
	}
	return o.Start.UTC().Format(time.RFC3339) + "|" + o.Settlement + "|" + p
}

func norm(ps []string) string {
	n := make([]string, len(ps))
	for i, p := range ps {
		// quotes and spaces are ignored: Notices space and quote names inconsistently
		n[i] = strings.ToLower(strings.Join(strings.Fields(strings.NewReplacer(`"`, "", "'", "").Replace(p)), ""))
	}
	sort.Strings(n)
	return strings.Join(n, "\x00")
}

// Compare scores got against want. Outages are paired in start order; each
// expected Outage counts once per field, a missing one counts wrong everywhere.
// "outages" scores one point per Notice: whether the count matches.
func Compare(want []Outage, got []Outage) Score {
	s := Score{}
	s.add("outages", len(want) == len(got))
	w := append([]Outage(nil), want...)
	g := append([]Outage(nil), got...)
	sort.Slice(w, func(i, j int) bool { return key(w[i]) < key(w[j]) })
	sort.Slice(g, func(i, j int) bool { return key(g[i]) < key(g[j]) })
	for i, e := range w {
		var a Outage
		have := i < len(g)
		if have {
			a = g[i]
		}
		s.add("start", have && a.Start.Equal(e.Start))
		s.add("end", have && a.End.Equal(e.End))
		s.add("end_approximate", have && a.EndApproximate == e.EndApproximate)
		s.add("severity", have && a.Severity == e.Severity)
		s.add("kind", have && a.Kind == e.Kind)
		s.add("settlement", have && a.Settlement == e.Settlement)
		s.add("places", have && norm(a.Places) == norm(e.Places))
	}
	return s
}

// FromIngest converts extractor output.
func FromIngest(os []ingest.Outage) []Outage {
	out := []Outage{}
	for _, o := range os {
		out = append(out, Outage{o.Start, o.End, o.EndApproximate, o.Severity, o.Kind, o.Settlement, o.Places})
	}
	return out
}

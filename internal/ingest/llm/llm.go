// Package llm is the shared LLM extractor: Notice text in, Outages out, behind
// the same one-operation shape as the Providers' deterministic parsers.
package llm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	_ "time/tzdata" // Europe/Sofia must resolve in a bare container

	"github.com/alisa/outage-radar/internal/ingest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// shortcut: model ID not checked against the live API (never called outside tests); verify before deploying.
const model = "claude-haiku-5-5"

const system = `You extract water or power outages from one Bulgarian utility Notice. ` +
	`Call record_outages with one entry per interruption (per time window). ` +
	`"date" is the day named in the text as DD.MM or DD.MM.YYYY, or "" if the text names none. ` +
	`"start" and "end" are 24-hour HH:MM. Set end_approximate when the end is given as "около". ` +
	`severity is "cut" for no water or power and "disturbance" for possible disturbances only. ` +
	`"settlement" is the town or village (Варна if unnamed); "places" are the streets, quarters or areas affected, quoted from the text. ` +
	`Return an empty list if the Notice announces no interruption.`

var schema = json.RawMessage(`{"type":"object","required":["outages"],"properties":{"outages":{"type":"array","items":{
	"type":"object","required":["date","start","end","end_approximate","severity","settlement","places"],"properties":{
	"date":{"type":"string"},"start":{"type":"string"},"end":{"type":"string"},"end_approximate":{"type":"boolean"},
	"severity":{"type":"string","enum":["cut","disturbance"]},"settlement":{"type":"string"},
	"places":{"type":"array","items":{"type":"string"}}}}}}}`)

type Extractor struct {
	pool    *pgxpool.Pool
	baseURL string
	apiKey  string
	client  *http.Client
}

func New(pool *pgxpool.Pool, baseURL, apiKey string) *Extractor {
	return &Extractor{pool, strings.TrimRight(baseURL, "/"), apiKey, &http.Client{Timeout: 60 * time.Second}}
}

type raw struct {
	Outages []struct {
		Date           string   `json:"date"`
		Start          string   `json:"start"`
		End            string   `json:"end"`
		EndApproximate bool     `json:"end_approximate"`
		Severity       string   `json:"severity"`
		Settlement     string   `json:"settlement"`
		Places         []string `json:"places"`
	} `json:"outages"`
}

// Extract returns Outages without Kind (the Provider's list decides that).
// Failures are returned, never turned into Outages, so the Notice stays queued.
func (e *Extractor) Extract(n ingest.Notice) ([]ingest.Outage, error) {
	ctx := context.Background()
	sum := sha256.Sum256([]byte(n.Text))
	hash := hex.EncodeToString(sum[:])
	var body []byte
	err := e.pool.QueryRow(ctx, `SELECT result FROM llm_extractions WHERE text_hash = $1`, hash).Scan(&body)
	if err == pgx.ErrNoRows {
		if body, err = e.ask(ctx, n.Text); err != nil {
			return nil, err
		}
		if _, err = resolve(body, n.Published); err != nil { // never cache invalid output
			return nil, err
		}
		_, err = e.pool.Exec(ctx, `INSERT INTO llm_extractions (text_hash, result) VALUES ($1, $2) ON CONFLICT DO NOTHING`, hash, body)
	}
	if err != nil {
		return nil, err
	}
	return resolve(body, n.Published)
}

func (e *Extractor) ask(ctx context.Context, text string) ([]byte, error) {
	req, _ := json.Marshal(map[string]any{
		"model": model, "max_tokens": 2048, "system": system,
		"tools":       []any{map[string]any{"name": "record_outages", "description": "Record the outages in the Notice.", "input_schema": schema}},
		"tool_choice": map[string]any{"type": "tool", "name": "record_outages"},
		"messages":    []any{map[string]any{"role": "user", "content": text}}, // Notice text only
	})
	r, _ := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/v1/messages", bytes.NewReader(req))
	r.Header.Set("x-api-key", e.apiKey)
	r.Header.Set("anthropic-version", "2023-06-01")
	r.Header.Set("content-type", "application/json")
	resp, err := e.client.Do(r)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("claude: status %d", resp.StatusCode)
	}
	var out struct {
		Content []struct {
			Type  string          `json:"type"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("claude: %w", err)
	}
	for _, c := range out.Content {
		if c.Type == "tool_use" && c.Name == "record_outages" {
			return c.Input, nil
		}
	}
	return nil, fmt.Errorf("claude: no record_outages call")
}

var sofia = func() *time.Location {
	l, err := time.LoadLocation("Europe/Sofia")
	if err != nil {
		panic(err)
	}
	return l
}()

// resolve validates the model's output and turns it into Outages in Europe/Sofia;
// a missing date is the Notice's publication date.
func resolve(body []byte, published time.Time) ([]ingest.Outage, error) {
	var r raw
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("invalid extraction: %w", err)
	}
	pub := published.In(sofia)
	out := []ingest.Outage{}
	for _, o := range r.Outages {
		day := time.Date(pub.Year(), pub.Month(), pub.Day(), 0, 0, 0, 0, sofia)
		if o.Date != "" {
			d, err := time.ParseInLocation("02.01.2006", o.Date, sofia)
			if err != nil {
				if d, err = time.ParseInLocation("02.01", o.Date, sofia); err != nil {
					return nil, fmt.Errorf("invalid date %q", o.Date)
				}
				d = d.AddDate(pub.Year(), 0, 0)
				if d.Before(day.AddDate(0, 0, -180)) { // "02.01" in a December Notice
					d = d.AddDate(1, 0, 0)
				}
			}
			day = d
		}
		s, e := hm(day, o.Start), hm(day, o.End)
		if s.IsZero() || e.IsZero() || (o.Severity != "cut" && o.Severity != "disturbance") {
			return nil, fmt.Errorf("invalid outage %+v", o)
		}
		if !e.After(s) {
			e = e.AddDate(0, 0, 1)
		}
		if o.Settlement == "" {
			o.Settlement = "Варна"
		}
		out = append(out, ingest.Outage{Start: s, End: e, EndApproximate: o.EndApproximate, Severity: o.Severity, Settlement: o.Settlement, Places: o.Places})
	}
	return out, nil
}

func hm(day time.Time, s string) time.Time {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return time.Time{}
	}
	return time.Date(day.Year(), day.Month(), day.Day(), t.Hour(), t.Minute(), 0, 0, sofia)
}

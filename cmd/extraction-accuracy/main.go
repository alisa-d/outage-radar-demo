// Command extraction-accuracy runs the LLM extractor over the labelled Notice set
// (internal/ingest/accuracy/testdata) and prints accuracy per field and the token
// cost. It spends real money, so it is on demand only: never wire it into CI.
//
//	ANTHROPIC_API_KEY=... go run ./cmd/extraction-accuracy [-dir DIR] [-model ID] [-v]
//
// Optional ACCURACY_USD_PER_MTOK_IN and ACCURACY_USD_PER_MTOK_OUT turn tokens into dollars.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/alisa/outage-radar/internal/ingest/accuracy"
	"github.com/alisa/outage-radar/internal/ingest/llm"
)

func main() {
	dir := flag.String("dir", "internal/ingest/accuracy/testdata", "labelled Notices")
	model := flag.String("model", llm.Model, "model ID to test")
	verbose := flag.Bool("v", false, "list every Notice that is not fully right")
	flag.Parse()
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		fmt.Println("SKIPPED: ANTHROPIC_API_KEY is not set; the LLM accuracy run costs real money and only runs on demand with a key.")
		return
	}
	base := os.Getenv("ANTHROPIC_BASE_URL")
	if base == "" {
		base = "https://api.anthropic.com"
	}
	cases, err := accuracy.Load(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	client := &http.Client{Timeout: 60 * time.Second}
	total, ran, failed := accuracy.Score{}, 0, 0
	var use llm.Usage
	for _, c := range cases {
		if c.Provider != "ВиК Варна" { // ЕРП Север's Notices are parsed deterministically, never sent to the LLM
			continue
		}
		n := c.IngestNotice()
		body, u, err := llm.Ask(context.Background(), client, base, key, *model, n.Text)
		use.InputTokens += u.InputTokens
		use.OutputTokens += u.OutputTokens
		var got []accuracy.Outage
		if err == nil {
			outs, rerr := llm.Resolve(body, n.Published)
			got, err = accuracy.FromIngest(outs), rerr
		}
		if err != nil {
			fmt.Printf("%s: %v (counted wrong)\n", c.Name, err)
			failed++
		}
		for i := range got {
			got[i].Kind = c.Expected[0].Kind // the Provider's list decides Kind, not the model
		}
		s := accuracy.Compare(c.Expected, got)
		total.Add(s)
		ran++
		if *verbose && !s.Perfect() {
			fmt.Printf("%s\n  want %+v\n  got  %+v\n", c.Name, c.Expected, got)
		}
	}
	fmt.Printf("model %s, %d Notices (%d request failures)\n%s", *model, ran, failed, total)
	fmt.Printf("tokens: %d in, %d out\n", use.InputTokens, use.OutputTokens)
	pin, e1 := strconv.ParseFloat(os.Getenv("ACCURACY_USD_PER_MTOK_IN"), 64)
	pout, e2 := strconv.ParseFloat(os.Getenv("ACCURACY_USD_PER_MTOK_OUT"), 64)
	if e1 == nil && e2 == nil {
		fmt.Printf("cost: $%.4f\n", (float64(use.InputTokens)*pin+float64(use.OutputTokens)*pout)/1e6)
	}
}

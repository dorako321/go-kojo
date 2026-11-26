package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dorako321/go-kojo/internal/engine"
	"github.com/dorako321/go-kojo/internal/factory"
	"github.com/dorako321/go-kojo/internal/order"
)

// Arm is one way of staffing the factory.
type Arm struct {
	Name   string
	Desk   order.Models
	Build  string
	Rescue string
}

type OrderConfig struct {
	Orders   []string // what a customer would type
	Arms     []Arm
	Parallel int
	OutDir   string
	Engine   engine.Engine
	Log      io.Writer
}

// OrderRow is one order taken from the customer's words to a product.
type OrderRow struct {
	Order     string       `json:"order"`
	Arm       string       `json:"arm"`
	Status    string       `json:"status"` // ok | failed | rejected | cannot | dispute | error
	Name      string       `json:"name,omitempty"`
	Questions int          `json:"questions"`
	Reviewed  int          `json:"reviewed"`
	Disputed  []string     `json:"disputed,omitempty"`
	Revised   bool         `json:"revised"`
	Repairs   int          `json:"repairs"`
	Rescued   bool         `json:"rescued"` // the last repair went to the rescue model
	Seconds   float64      `json:"seconds"`
	CostUSD   float64      `json:"cost_usd"`
	Failed    []string     `json:"failed_gates,omitempty"`
	Detail    string       `json:"detail,omitempty"`
	Review    string       `json:"review,omitempty"` // what the reviewer disputed, as told to the designer
	Calls     []order.Call `json:"calls"`            // every use of a model, in order
}

// RunOrders sends every order through every arm. A question from reception
// is answered with its first option, the one it recommends.
func RunOrders(ctx context.Context, cfg OrderConfig) ([]OrderRow, error) {
	out, err := filepath.Abs(cfg.OutDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return nil, err
	}
	results, err := os.OpenFile(filepath.Join(out, "orders.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	defer results.Close()

	var (
		mu   sync.Mutex
		rows []OrderRow
		wg   sync.WaitGroup
		sem  = make(chan struct{}, max(cfg.Parallel, 1))
	)
	for i, text := range cfg.Orders {
		for _, arm := range cfg.Arms {
			wg.Add(1)
			go func() {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				row := runOrder(ctx, cfg.Engine, arm, text, filepath.Join(out, fmt.Sprintf("%s-%02d", arm.Name, i+1)))

				mu.Lock()
				defer mu.Unlock()
				rows = append(rows, row)
				line, _ := json.Marshal(row)
				results.Write(append(line, '\n'))
				fmt.Fprintf(cfg.Log, "%-5s %02d %-9s %-10s q=%d reviewed=%d disputed=%d repairs=%d %4.0fs $%.3f  %s\n",
					arm.Name, i+1, row.Status, row.Name, row.Questions, row.Reviewed, len(row.Disputed), row.Repairs, row.Seconds, row.CostUSD, clipLine(row.Detail))
			}()
		}
	}
	wg.Wait()

	fmt.Fprintf(cfg.Log, "\n%-6s %8s %10s %9s %8s %8s\n", "arm", "shipped", "first try", "disputes", "avg sec", "avg $")
	for _, arm := range cfg.Arms {
		var n, ok, first, disputes int
		var sec, cost float64
		for _, r := range rows {
			if r.Arm != arm.Name {
				continue
			}
			n++
			sec += r.Seconds
			cost += r.CostUSD
			disputes += len(r.Disputed)
			if r.Status == "ok" {
				ok++
				if r.Repairs == 0 {
					first++
				}
			}
		}
		if n > 0 {
			fmt.Fprintf(cfg.Log, "%-6s %5d/%-2d %7d/%-2d %9d %8.0f %8.3f\n", arm.Name, ok, n, first, n, disputes, sec/float64(n), cost/float64(n))
		}
	}
	return rows, nil
}

func runOrder(ctx context.Context, eng engine.Engine, arm Arm, text, dir string) (row OrderRow) {
	start := time.Now()
	row = OrderRow{Order: text, Arm: arm.Name, Status: "error"}
	defer func() { row.Seconds = time.Since(start).Seconds() }()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		row.Detail = err.Error()
		return row
	}

	rep, err := order.Auto(ctx, eng, arm.Desk, text, dir, nil)
	row.CostUSD, row.Calls, row.Questions = rep.CostUSD, rep.Calls, rep.Questions
	row.Reviewed, row.Disputed, row.Revised, row.Review = rep.Reviewed, rep.Disputed, rep.Revised, rep.Dispute
	if err != nil {
		row.Detail = err.Error()
		return row
	}
	if rep.Action == "reject" {
		row.Status, row.Detail = "rejected", rep.Message
		return row
	}
	{
		row.Name = rep.Spec.Name
		specFile, _ := json.MarshalIndent(rep.Spec, "", "  ")
		os.WriteFile(filepath.Join(dir, "spec.json"), specFile, 0o644)
		built, err := factory.Build(ctx, *rep.Spec, filepath.Join(dir, rep.Spec.Name), factory.Options{
			Engine: eng, Model: arm.Build, Rescue: arm.Rescue, Owner: "example", Holder: "go-kojo eval",
			Request: text, MaxRepairs: 2, RunDir: filepath.Join(dir, "run"),
		})
		row.CostUSD += built.CostUSD
		for _, a := range built.Attempts {
			row.Calls = append(row.Calls, order.Call{Role: a.Kind, Model: a.Model, Seconds: a.Seconds, CostUSD: a.CostUSD})
		}
		row.Status, row.Repairs, row.Failed, row.Detail = built.Status, built.Repairs(), built.FailedGates(), built.Reason
		if n := len(built.Attempts); n > 0 && arm.Rescue != "" && arm.Rescue != arm.Build {
			row.Rescued = built.Attempts[n-1].Model == arm.Rescue
		}
		if err != nil {
			row.Status = "error"
		}
		return row
	}
}

func clipLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len([]rune(s)) > 70 {
		return string([]rune(s)[:70]) + "…"
	}
	return s
}

package resources

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/qiangli/coreutils/pkg/fleet"
	"github.com/qiangli/coreutils/pkg/otelquery"
	"github.com/qiangli/coreutils/pkg/weave"
)

// Canonical provider names in requested order.
var CanonicalProviders = []string{
	"Anthropic",
	"OpenAI",
	"Google",
	"Zhipu",
	"Moonshot",
	"DeepSeek",
}

// FleetGroup represents resource utilization for one Provider and Band.
type FleetGroup struct {
	Provider     string   `json:"provider"`
	Band         string   `json:"band"`
	BandNum      int      `json:"band_num"`
	Total        int      `json:"total"`
	Busy         int      `json:"busy"`
	Idle         int      `json:"idle"`
	Cooling      int      `json:"cooling"`
	Unavailable  int      `json:"unavailable"`
	Subscription int      `json:"subscription"`
	APIKey       int      `json:"api_key"`
	Tokens       *int64   `json:"tokens,omitempty"`
	CostUSD      *float64 `json:"cost_usd,omitempty"`
	MeterPresent bool     `json:"meter_present"`
}

// FleetTotals holds aggregate utilization stats across all groups.
type FleetTotals struct {
	Total        int      `json:"total"`
	Busy         int      `json:"busy"`
	Idle         int      `json:"idle"`
	Cooling      int      `json:"cooling"`
	Unavailable  int      `json:"unavailable"`
	Subscription int      `json:"subscription"`
	APIKey       int      `json:"api_key"`
	Tokens       *int64   `json:"tokens,omitempty"`
	CostUSD      *float64 `json:"cost_usd,omitempty"`
	MeterPresent bool     `json:"meter_present"`
}

// FleetResources represents the complete `bashy resources fleet` envelope.
type FleetResources struct {
	SchemaVersion string       `json:"schema_version"`
	GeneratedAt   time.Time    `json:"generated_at"`
	Groups        []FleetGroup `json:"groups"`
	Totals        FleetTotals  `json:"totals"`
	MeterPresent  bool         `json:"meter_present"`
}

type BoardAgent struct {
	Name         string
	Tool         string
	Model        string
	Band         int
	Available    bool
	Found        bool
	Availability string
	State        string
}

type BoardRun struct {
	State string
	Tool  string
	Agent string
	Model string
}

type wireEnvelope struct {
	Status string          `json:"status"`
	Result json.RawMessage `json:"result"`
}

// CanonicalProvider maps model/provider names to the six canonical providers.
func CanonicalProvider(modelName, providerStr string) string {
	prov := strings.ToLower(providerStr)
	mod := strings.ToLower(modelName)

	switch {
	case strings.Contains(prov, "anthropic") || strings.Contains(mod, "claude") || strings.Contains(mod, "fable") || strings.Contains(mod, "haiku") || strings.Contains(mod, "opus") || strings.Contains(mod, "sonnet"):
		return "Anthropic"
	case (strings.Contains(prov, "openai") && !strings.Contains(prov, "openai-compat")) || strings.Contains(mod, "gpt") || strings.Contains(mod, "codex"):
		return "OpenAI"
	case strings.Contains(prov, "google") || strings.Contains(prov, "gemini") || strings.Contains(mod, "gemini") || strings.Contains(mod, "agy"):
		return "Google"
	case strings.Contains(prov, "zhipu") || strings.Contains(prov, "glm") || strings.Contains(mod, "glm") || strings.Contains(prov, "z.ai"):
		return "Zhipu"
	case strings.Contains(prov, "moonshot") || strings.Contains(prov, "kimi") || strings.Contains(mod, "kimi") || strings.Contains(mod, "moonshot"):
		return "Moonshot"
	case strings.Contains(prov, "deepseek") || strings.Contains(mod, "deepseek"):
		return "DeepSeek"
	default:
		if providerStr != "" && providerStr != "openai-compat" {
			return strings.Title(providerStr)
		}
		return "Other"
	}
}

// CollectFleetResources gathers live weave availability, active run counts,
// fleet catalog metadata, and OTel cost/token metrics.
func CollectFleetResources(ctx context.Context) (*FleetResources, error) {
	return CollectFleetResourcesFromBoard(ctx, time.Time{}, nil, nil)
}

type liveAvailInfo struct {
	found        bool
	available    bool
	reason       string
	coolingUntil string
}

func normStr(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "_", "")
	s = strings.ReplaceAll(s, ".", "")
	return s
}

type agentRecord struct {
	Name        string
	Tool        string
	Model       string
	Provider    string
	Band        int
	Kind        string
	BillingMode string
	Aliases     []string
}

// CollectFleetResourcesFromBoard builds FleetResources from provided board agents/runs,
// or queries the live catalog and weave state if nil.
func CollectFleetResourcesFromBoard(ctx context.Context, at time.Time, bAgents []BoardAgent, bRuns []BoardRun) (*FleetResources, error) {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	cat := fleet.New()

	availMap := map[string]liveAvailInfo{}
	busyMap := map[string]int{}

	markBusy := func(keys ...string) {
		for _, k := range keys {
			if k != "" {
				busyMap[k]++
				busyMap[normStr(k)]++
			}
		}
	}

	var records []agentRecord

	if bAgents != nil {
		for _, a := range bAgents {
			availMap[a.Name] = liveAvailInfo{
				found:     a.Found,
				available: a.Available,
				reason:    a.Availability,
				coolingUntil: func() string {
					if strings.HasPrefix(a.Availability, "cooling") {
						return a.Availability
					}
					return ""
				}(),
			}
			if a.State == "working" {
				markBusy(a.Name, a.Tool, a.Model)
			}

			// Try resolving from catalog for richer metadata
			rec := agentRecord{
				Name:  a.Name,
				Tool:  a.Tool,
				Model: a.Model,
				Band:  a.Band,
			}
			if resolved, _, m, err := cat.Binding(a.Name); err == nil {
				rec.Tool = resolved.Tool
				rec.Model = m.Name
				rec.Provider = m.Provider
				if rec.Band <= 0 {
					rec.Band = m.Band
				}
				rec.Kind = m.Kind
				rec.BillingMode = m.BillingMode()
				rec.Aliases = resolved.Names()
			}
			records = append(records, rec)
		}
		for _, r := range bRuns {
			if r.State == "working" || r.State == "allocated" {
				markBusy(r.Agent, r.Tool, r.Model)
			}
		}
	} else {
		// Live availability from `weave fleet --agents --json`
		type availability struct {
			Agent        string `json:"agent"`
			Tool         string `json:"tool"`
			Model        string `json:"model"`
			Reason       string `json:"reason"`
			CoolingUntil string `json:"cooling_until"`
			Available    bool   `json:"available"`
			Found        bool   `json:"found"`
		}
		rawAvail, err := runCobraJSON("fleet", "--agents", "--json")
		if err == nil {
			var env wireEnvelope
			var res struct {
				Tools []availability `json:"tools"`
			}
			if json.Unmarshal(rawAvail, &env) == nil && json.Unmarshal(env.Result, &res) == nil {
				for _, row := range res.Tools {
					availMap[row.Agent] = liveAvailInfo{
						found:        row.Found,
						available:    row.Available,
						reason:       row.Reason,
						coolingUntil: row.CoolingUntil,
					}
				}
			}
		}

		// Active runs from `weave list --all --json`
		rawRuns, err := runCobraJSON("list", "--all", "--json")
		if err == nil {
			var env wireEnvelope
			var res struct {
				Queues []struct {
					Items []struct {
						State string `json:"state"`
						Tool  string `json:"tool"`
						Owner string `json:"owner"`
						Model string `json:"model"`
						Launch *struct {
							Agent string `json:"agent"`
							Model string `json:"model"`
						} `json:"launch_spec"`
					} `json:"items"`
				} `json:"queues"`
			}
			if json.Unmarshal(rawRuns, &env) == nil && json.Unmarshal(env.Result, &res) == nil {
				for _, q := range res.Queues {
					for _, x := range q.Items {
						if x.State == "working" || x.State == "allocated" {
							agentName := x.Owner
							modelName := x.Model
							if x.Launch != nil {
								if x.Launch.Agent != "" {
									agentName = x.Launch.Agent
								}
								if x.Launch.Model != "" {
									modelName = x.Launch.Model
								}
							}
							markBusy(agentName, x.Tool, modelName)
						}
					}
				}
			}
		}

		agents, errs := cat.Agents()
		if len(errs) > 0 {
			return nil, errs[0]
		}
		for _, a := range agents {
			_, t, m, _ := cat.Binding(a.Name)
			bNum := a.Band
			if bNum <= 0 {
				bNum = m.Band
			}
			records = append(records, agentRecord{
				Name:        a.Name,
				Tool:        t.Name,
				Model:       m.Name,
				Provider:    m.Provider,
				Band:        bNum,
				Kind:        m.Kind,
				BillingMode: m.BillingMode(),
				Aliases:     a.Names(),
			})
		}
	}

	// OTel metric store
	otelClient := otelquery.NewClient("")
	meterPresent := otelClient.Reachable(ctx)
	modelTokens := map[string]int64{}
	modelCosts := map[string]float64{}

	if meterPresent {
		if series, _, err := otelClient.Metrics(ctx, `sum(ycode.llm.tokens.total) by (model)`); err == nil {
			for _, s := range series {
				modelTokens[s.Labels["model"]] = int64(s.Value)
			}
		}
		if series, _, err := otelClient.Metrics(ctx, `sum(agent.turn.tokens) by (model)`); err == nil {
			for _, s := range series {
				modelTokens[s.Labels["model"]] += int64(s.Value)
			}
		}
		if series, _, err := otelClient.Metrics(ctx, `sum(ycode.llm.cost.dollars) by (model)`); err == nil {
			for _, s := range series {
				modelCosts[s.Labels["model"]] = s.Value
			}
		}
		if series, _, err := otelClient.Metrics(ctx, `sum(fleet.cost) by (model)`); err == nil {
			for _, s := range series {
				modelCosts[s.Labels["model"]] += s.Value
			}
		}
	}

	type groupKey struct {
		provider string
		bandNum  int
	}
	groupsMap := map[groupKey]*FleetGroup{}

	for _, a := range records {
		provName := CanonicalProvider(a.Model, a.Provider)

		bNum := a.Band
		if bNum <= 0 {
			bNum = 1
		}

		key := groupKey{provider: provName, bandNum: bNum}
		grp, ok := groupsMap[key]
		if !ok {
			grp = &FleetGroup{
				Provider:     provName,
				Band:         fmt.Sprintf("L%d", bNum),
				BandNum:      bNum,
				MeterPresent: meterPresent,
			}
			groupsMap[key] = grp
		}

		billing := a.BillingMode
		isSub := a.Kind == fleet.ModelKindSubscription || billing == fleet.BillingFlat || billing == fleet.BillingFlatThenMetered
		if isSub {
			grp.Subscription++
		} else {
			grp.APIKey++
		}

		// Alias / name resolution for live availability
		names := append([]string{a.Name}, a.Aliases...)
		live, hasLive := availMap[a.Name]
		if !hasLive {
			for _, name := range names {
				if l, ok := availMap[name]; ok {
					live, hasLive = l, true
					break
				}
			}
		}

		isFound := true
		isAvail := true
		isCooling := false

		if hasLive {
			isFound = live.found
			isAvail = live.available
			if live.coolingUntil != "" || strings.Contains(live.reason, "cooling") {
				isCooling = true
			}
		} else if bAgents == nil {
			binary := a.Tool
			_, lookErr := exec.LookPath(binary)
			isFound = lookErr == nil
			isAvail = isFound
		}

		// Check busy state by agent name, aliases, or model name
		isBusy := false
		for _, name := range names {
			if busyMap[name] > 0 || busyMap[normStr(name)] > 0 {
				isBusy = true
				break
			}
		}
		if !isBusy {
			if busyMap[a.Model] > 0 || busyMap[normStr(a.Model)] > 0 {
				isBusy = true
			}
		}

		grp.Total++
		switch {
		case isBusy:
			grp.Busy++
		case isCooling:
			grp.Cooling++
		case !isAvail || !isFound:
			grp.Unavailable++
		default:
			grp.Idle++
		}

		if meterPresent {
			if tok, hasTok := modelTokens[a.Model]; hasTok {
				if grp.Tokens == nil {
					var zero int64
					grp.Tokens = &zero
				}
				*grp.Tokens += tok
			}
			if cost, hasCost := modelCosts[a.Model]; hasCost {
				if grp.CostUSD == nil {
					var zero float64
					grp.CostUSD = &zero
				}
				*grp.CostUSD += cost
			}
		}
	}

	provOrder := map[string]int{}
	for idx, p := range CanonicalProviders {
		provOrder[p] = idx
	}

	var groupKeys []groupKey
	for k := range groupsMap {
		groupKeys = append(groupKeys, k)
	}

	sort.Slice(groupKeys, func(i, j int) bool {
		pi, okI := provOrder[groupKeys[i].provider]
		if !okI {
			pi = 99
		}
		pj, okJ := provOrder[groupKeys[j].provider]
		if !okJ {
			pj = 99
		}
		if pi != pj {
			return pi < pj
		}
		if groupKeys[i].provider != groupKeys[j].provider {
			return groupKeys[i].provider < groupKeys[j].provider
		}
		return groupKeys[i].bandNum < groupKeys[j].bandNum
	})

	var resultGroups []FleetGroup
	var totals FleetTotals
	totals.MeterPresent = meterPresent

	for _, k := range groupKeys {
		g := *groupsMap[k]
		resultGroups = append(resultGroups, g)

		totals.Total += g.Total
		totals.Busy += g.Busy
		totals.Idle += g.Idle
		totals.Cooling += g.Cooling
		totals.Unavailable += g.Unavailable
		totals.Subscription += g.Subscription
		totals.APIKey += g.APIKey

		if meterPresent {
			if g.Tokens != nil {
				if totals.Tokens == nil {
					var zero int64
					totals.Tokens = &zero
				}
				*totals.Tokens += *g.Tokens
			}
			if g.CostUSD != nil {
				if totals.CostUSD == nil {
					var zero float64
					totals.CostUSD = &zero
				}
				*totals.CostUSD += *g.CostUSD
			}
		}
	}

	return &FleetResources{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   at,
		Groups:        resultGroups,
		Totals:        totals,
		MeterPresent:  meterPresent,
	}, nil
}

func runCobraJSON(args ...string) ([]byte, error) {
	cmd := weave.NewWeaveCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// FormatTable renders the text table output for `bashy resources fleet`.
func FormatTable(fr *FleetResources) string {
	var out bytes.Buffer
	w := tabwriter.NewWriter(&out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PROVIDER\tBAND\tTOTAL\tBUSY\tIDLE\tCOOLING\tUNAVAIL\tSUB\tAPI\tTOKENS\tCOST")

	for _, g := range fr.Groups {
		tokStr, costStr := "N/A", "N/A"
		if g.MeterPresent && g.Tokens != nil && g.CostUSD != nil {
			tokStr = formatTokens(*g.Tokens)
			costStr = fmt.Sprintf("$%.4f", *g.CostUSD)
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%s\t%s\n",
			g.Provider, g.Band, g.Total, g.Busy, g.Idle, g.Cooling, g.Unavailable,
			g.Subscription, g.APIKey, tokStr, costStr)
	}

	tokTot, costTot := "N/A", "N/A"
	if fr.MeterPresent && fr.Totals.Tokens != nil && fr.Totals.CostUSD != nil {
		tokTot = formatTokens(*fr.Totals.Tokens)
		costTot = fmt.Sprintf("$%.4f", *fr.Totals.CostUSD)
	}
	fmt.Fprintf(w, "Totals\t\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%s\t%s\n",
		fr.Totals.Total, fr.Totals.Busy, fr.Totals.Idle, fr.Totals.Cooling, fr.Totals.Unavailable,
		fr.Totals.Subscription, fr.Totals.APIKey, tokTot, costTot)

	_ = w.Flush()
	return out.String()
}

func formatTokens(n int64) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000.0)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fk", float64(n)/1_000.0)
	}
	return strconv.FormatInt(n, 10)
}

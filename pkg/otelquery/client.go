package otelquery

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewClient(baseURL string) *Client {
	if baseURL == "" {
		baseURL = os.Getenv("BASHY_OTEL_QUERY_URL")
	}
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) logRows(ctx context.Context, backendPath, query string, since time.Duration, limit int) ([]map[string]any, string, error) {
	u, err := url.Parse(c.BaseURL + backendPath)
	if err != nil {
		return nil, "", err
	}
	q := u.Query()
	q.Set("query", query)
	q.Set("limit", strconv.Itoa(limit))
	if since > 0 {
		end := time.Now()
		q.Set("start", end.Add(-since).UTC().Format(time.RFC3339Nano))
		q.Set("end", end.UTC().Format(time.RFC3339Nano))
	}
	u.RawQuery = q.Encode()
	body, err := c.get(ctx, u.String())
	if err != nil {
		return nil, u.String(), err
	}
	rows, err := decodeLogRows(body)
	return rows, u.String(), err
}

func (c *Client) metricQuery(ctx context.Context, promql string) ([]MetricSeries, string, error) {
	u, err := url.Parse(c.BaseURL + "/metrics/api/v1/query")
	if err != nil {
		return nil, "", err
	}
	q := u.Query()
	q.Set("query", promql)
	u.RawQuery = q.Encode()
	body, err := c.get(ctx, u.String())
	if err != nil {
		return nil, u.String(), err
	}
	series, err := decodeMetricSeries(body)
	return series, u.String(), err
}

func (c *Client) jaegerTrace(ctx context.Context, traceID string) ([]map[string]any, string, error) {
	u := c.BaseURL + "/traces/select/jaeger/api/traces/" + url.PathEscape(traceID)
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, u, err
	}
	rows, err := decodeJaegerTrace(body)
	return rows, u, err
}

func (c *Client) get(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("backend unreachable: %w", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(b))
		if msg == "" {
			msg = resp.Status
		}
		return nil, fmt.Errorf("backend returned %s: %s", resp.Status, msg)
	}
	return b, nil
}

func decodeLogRows(b []byte) ([]map[string]any, error) {
	var wrapped struct {
		Data []map[string]any `json:"data"`
		Hits []map[string]any `json:"hits"`
	}
	if json.Unmarshal(b, &wrapped) == nil {
		if wrapped.Data != nil {
			return wrapped.Data, nil
		}
		if wrapped.Hits != nil {
			return wrapped.Hits, nil
		}
	}
	var arr []map[string]any
	if json.Unmarshal(b, &arr) == nil {
		return arr, nil
	}
	var rows []map[string]any
	sc := bufio.NewScanner(strings.NewReader(string(b)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var row map[string]any
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, sc.Err()
}

func decodeMetricSeries(b []byte) ([]MetricSeries, error) {
	var resp struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				Metric map[string]string `json:"metric"`
				Value  []any             `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		return nil, err
	}
	out := make([]MetricSeries, 0, len(resp.Data.Result))
	for _, r := range resp.Data.Result {
		v := 0.0
		if len(r.Value) > 1 {
			v = number(r.Value[1])
		}
		out = append(out, MetricSeries{Name: r.Metric["__name__"], Labels: r.Metric, Value: v})
	}
	return out, nil
}

func decodeJaegerTrace(b []byte) ([]map[string]any, error) {
	var resp struct {
		Data []struct {
			Spans []map[string]any `json:"spans"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		return nil, err
	}
	if len(resp.Data) == 0 {
		return nil, nil
	}
	return resp.Data[0].Spans, nil
}

func field(row map[string]any, names ...string) string {
	for _, n := range names {
		if v, ok := row[n]; ok {
			switch x := v.(type) {
			case string:
				return x
			case float64:
				if math.Trunc(x) == x {
					return strconv.FormatInt(int64(x), 10)
				}
				return strconv.FormatFloat(x, 'f', -1, 64)
			case bool:
				return strconv.FormatBool(x)
			default:
				if x != nil {
					return fmt.Sprint(x)
				}
			}
		}
	}
	return ""
}

func number(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return f
	default:
		f, _ := strconv.ParseFloat(fmt.Sprint(v), 64)
		return f
	}
}

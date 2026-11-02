package agent

// Optional OpenTelemetry export: each record of the turn log becomes a span, sent as OTLP/HTTP JSON to
// the otlpEndpoint of the user's settings. It is off unless that setting exists, a project file cannot
// set it, and it carries names, sizes and timings only, never prompts, inputs or outputs. A collector
// that is down costs a journal line, never the turn.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const otlpTimeout = 5 * time.Second

// otlpSlots bounds the exports in flight; a record that finds them full is dropped.
var otlpSlots = make(chan struct{}, 8)

func exportOTLP(s *Session, r TurnRecord) {
	endpoint := strings.TrimRight(s.settings.OTLPEndpoint, "/")
	if endpoint == "" {
		return
	}
	select {
	case otlpSlots <- struct{}{}:
	default:
		return
	}
	body, err := json.Marshal(otlpTraces(r))
	if err != nil {
		<-otlpSlots
		return
	}
	go func() {
		defer func() { <-otlpSlots }()
		if err := postOTLP(endpoint+"/v1/traces", body); err != nil {
			Journal(s.Root, JournalEntry{Kind: "gate", Detail: "telemetry export failed: " + firstLine(err.Error())})
		}
	}()
}

func postOTLP(url string, body []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), otlpTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("collector answered %d", resp.StatusCode)
	}
	return nil
}

func otlpString(k, v string) map[string]any {
	return map[string]any{"key": k, "value": map[string]any{"stringValue": v}}
}

func otlpInt(k string, v int) map[string]any {
	return map[string]any{"key": k, "value": map[string]any{"intValue": fmt.Sprint(v)}}
}

// otlpID derives a stable hex id: spans of one turn share a trace.
func otlpID(n int, parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:n])
}

func otlpTraces(r TurnRecord) map[string]any {
	name, attrs := otlpSpan(r)
	attrs = append(attrs, otlpString("infuse.session", r.Session), otlpInt("infuse.turn", r.Turn))
	end := r.TS * int64(time.Millisecond)
	span := map[string]any{
		"traceId": otlpID(16, r.Session, fmt.Sprint(r.Turn)), "spanId": otlpID(8, r.Session, fmt.Sprint(r.TS), name),
		"name": name, "kind": 1, "attributes": attrs,
		"startTimeUnixNano": fmt.Sprint(end - r.Ms*int64(time.Millisecond)), "endTimeUnixNano": fmt.Sprint(end),
	}
	return map[string]any{"resourceSpans": []any{map[string]any{
		"resource":   map[string]any{"attributes": []any{otlpString("service.name", "infuse")}},
		"scopeSpans": []any{map[string]any{"scope": map[string]any{"name": "infuse.turnlog"}, "spans": []any{span}}},
	}}}
}

// otlpSpan names a record's span and its attributes.
func otlpSpan(r TurnRecord) (string, []any) {
	if r.Tool != nil {
		return "tool " + r.Tool.Name, []any{otlpString("infuse.tool", r.Tool.Name), otlpInt("infuse.tool.output_bytes", r.Tool.OutputBytes)}
	}
	attrs := []any{otlpString("gen_ai.request.model", r.Model), otlpString("gen_ai.response.finish_reason", r.Stop)}
	if r.Usage != nil {
		attrs = append(attrs, otlpInt("gen_ai.usage.input_tokens", r.Usage.Input), otlpInt("gen_ai.usage.output_tokens", r.Usage.Output))
	}
	return "model " + r.Model, attrs
}

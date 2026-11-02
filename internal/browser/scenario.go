package browser

// A scripted run for the oracle's browser check: one private headless browser, one page, the declared
// steps, and what the page shows at the end. Nothing is shared with the agent's browser, and nothing
// here decides a verdict: the executor judges what comes back.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrNoBrowser means no Chromium-family browser could be started: the scenario was not measured.
var ErrNoBrowser = errors.New("no usable browser")

// ScenarioStep is one action on the page.
type ScenarioStep struct {
	Action   string `json:"action"`             // click | type | press
	Selector string `json:"selector,omitempty"` // click, type
	Text     string `json:"text,omitempty"`     // type: the text; press: the key
}

// Scenario is what to open and do.
type Scenario struct {
	URL      string
	Steps    []ScenarioStep
	Selector string // an element that must be on the final page ("" for none)
	Profile  string // a private, disposable browser profile folder
}

// ScenarioResult is what the page showed.
type ScenarioResult struct {
	Status     int // of the last document loaded, past any redirect; 0 when the browser saw none
	FinalURL   string
	Text       string
	Found      bool // Selector is on the final page
	Screenshot []byte
	Log        []string
}

const (
	scenarioWait   = 5 * time.Second // how long a selector may take to appear
	viewportWidth  = 1440
	viewportHeight = 900
	viewportScale  = 2
)

// RunScenario runs the steps. A failed step ends the scenario with an error and whatever the page
// showed; ErrNoBrowser reports that nothing could be measured.
func RunScenario(ctx context.Context, sc Scenario) (ScenarioResult, error) {
	var res ScenarioResult
	if findChromium() == "" {
		return res, ErrNoBrowser
	}
	b, err := startCDP(sc.Profile)
	if err != nil { // a first start on a fresh user profile can outlast the wait: once more
		b, err = startCDP(sc.Profile)
	}
	if err != nil {
		return res, fmt.Errorf("%w: %v", ErrNoBrowser, err)
	}
	defer b.close()
	p := &Page{b: b, netByID: map[string]*netEntry{}, x: -1, y: -1}
	b.mu.Lock()
	b.onEvent = p.event
	b.mu.Unlock()
	if _, err := b.call(ctx, "Network.enable", nil); err != nil {
		return res, err
	}
	p.setMetrics(viewportWidth, viewportHeight, viewportScale, false)
	if err := p.open(ctx, sc.URL); err != nil {
		return res, err
	}
	stepErr := p.runSteps(ctx, sc.Steps, &res)
	if stepErr == nil && sc.Selector != "" {
		_, err := p.locate(ctx, sc.Selector, scenarioWait)
		res.Found = err == nil
	}
	p.capture(ctx, &res)
	return res, stepErr
}

func (p *Page) open(ctx context.Context, url string) error {
	raw, err := p.b.call(ctx, "Page.navigate", map[string]any{"url": url})
	if err != nil {
		return err
	}
	var nav struct {
		ErrorText string `json:"errorText"`
	}
	_ = json.Unmarshal(raw, &nav)
	if nav.ErrorText != "" {
		return errors.New(nav.ErrorText)
	}
	p.b.waitLoad(ctx)
	return nil
}

func (p *Page) runSteps(ctx context.Context, steps []ScenarioStep, res *ScenarioResult) error {
	g := gestureStyle{stepPx: 1e9} // a check measures the page, not a hand
	for i, s := range steps {
		res.Log = append(res.Log, fmt.Sprintf("%d. %s %s %s", i+1, s.Action, s.Selector, s.Text))
		var err error
		switch s.Action {
		case "click", "type":
			var l *located
			if l, err = p.locate(ctx, s.Selector, scenarioWait); err == nil {
				p.click(ctx, g, l.X, l.Y, "left", 1)
				if s.Action == "type" {
					_ = p.press(ctx, "Control+A")
					p.typeText(ctx, g, s.Text)
				}
			}
		case "press":
			err = p.press(ctx, s.Text)
		default:
			err = fmt.Errorf("unknown step %q", s.Action)
		}
		if err != nil {
			return fmt.Errorf("step %d (%s %s): %w", i+1, s.Action, s.Selector, err)
		}
		sleepCtx(ctx, 200*time.Millisecond)
		p.b.waitLoad(ctx)
	}
	return nil
}

// capture reads the final page: the document's status, its address, its text and a screenshot.
func (p *Page) capture(ctx context.Context, res *ScenarioResult) {
	p.mu.Lock()
	for _, n := range p.net {
		if n.Type == "Document" && n.Status != 0 {
			res.Status = n.Status
		}
	}
	p.mu.Unlock()
	readString := func(js string) string {
		var s string
		if v, err := p.b.eval(ctx, js); err == nil {
			_ = json.Unmarshal(v, &s)
		}
		return s
	}
	res.FinalURL = readString("location.href")
	res.Text = strings.TrimSpace(readString("document.body ? document.body.innerText : ''"))
	raw, err := p.b.call(ctx, "Page.captureScreenshot", map[string]any{"format": "png"})
	if err != nil {
		return
	}
	var shot struct {
		Data string `json:"data"`
	}
	_ = json.Unmarshal(raw, &shot)
	res.Screenshot, _ = base64.StdEncoding.DecodeString(shot.Data)
}

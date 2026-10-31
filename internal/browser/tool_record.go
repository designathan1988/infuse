package browser

import (
	"fmt"
	"infuse/internal/core"
	"path/filepath"
	"strings"
	"time"
)

// Screen recordings.

func (a *action) record() (string, bool) {
	switch a.in.Action {
	case "record_start":
		a.p.startRecording()
		return a.h.Say(core.M("browser.recording")), false
	case "record_stop":
		dir := filepath.Join(a.h.RootDir(), core.InfuseDirName, "recordings", time.Now().Format("20060102-150405"))
		kfs, events, dur, total, err := a.p.stopRecording(dir, a.in.MaxFrames)
		if err != nil {
			return a.h.SayErr(err), true
		}
		secs := func(d time.Duration) string { return fmt.Sprintf("%.2f", d.Seconds()) }
		var b strings.Builder
		b.WriteString(a.h.Say(core.M("browser.rec_head", "seconds", fmt.Sprintf("%.1f", dur.Seconds()), "frames", total, "keyframes", len(kfs), "w", a.p.width, "h", a.p.height, "dir", core.RelTo(a.h.RootDir(), dir))) + "\n\n")
		event := func(e recEvent) {
			b.WriteString("    " + a.h.Say(core.M("browser.rec_event", "t", secs(e.at), "event", e.m)) + "\n")
		}
		ev := 0
		img := 0
		for i, k := range kfs {
			for ev < len(events) && events[ev].at <= k.at {
				event(events[ev])
				ev++
			}
			img++
			attachImage(a.h, k.jpeg)
			var line core.Msg
			switch {
			case i == 0:
				line = core.M("browser.rec_frame_start", "img", img, "frame", i+1, "t", secs(k.at))
			case len(k.regions) == 0:
				line = core.M("browser.rec_frame_final", "img", img, "frame", i+1, "t", secs(k.at))
			default:
				var where []string
				for _, r := range k.regions {
					where = append(where, fmt.Sprintf("x=%d y=%d %dx%d", r.Min.X, r.Min.Y, r.Dx(), r.Dy()))
				}
				line = core.M("browser.rec_frame_changed", "img", img, "frame", i+1, "t", secs(k.at), "pct", fmt.Sprintf("%.1f", k.changed*100), "areas", len(k.regions), "where", strings.Join(where, "; "))
			}
			b.WriteString(a.h.Say(line) + "\n")
			for ci, crop := range k.crops {
				img++
				attachImage(a.h, crop)
				b.WriteString(a.h.Say(core.M("browser.rec_crop", "img", img, "area", ci+1, "frame", i+1)) + "\n")
			}
		}
		for ; ev < len(events); ev++ {
			event(events[ev])
		}
		return b.String(), false
	}
	return "", true
}

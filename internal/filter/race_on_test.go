//go:build race

package filter

// The race detector multiplies memory-heavy code by an order of magnitude: the latency budget measures
// the filter, not the detector.
const latencyScale = 10

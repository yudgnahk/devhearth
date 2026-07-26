package trend

import (
	"fmt"
	"sort"
	"time"
)

// Direction classifies a series over the compared window.
const (
	DirectionGrowing   = "growing"
	DirectionShrinking = "shrinking"
	DirectionStable    = "stable"
)

// Regression kinds. Each names an observation, never a cause: the engine can
// see that a series jumped, not why it jumped.
const (
	RegressionSuddenGrowth    = "sudden_growth"
	RegressionSustainedGrowth = "sustained_growth"
	RegressionAdviceGrowth    = "advice_growth"
)

// Severity levels for a regression signal.
const (
	SeverityNotice  = "notice"
	SeverityWarning = "warning"
)

// Defaults chosen so a normal working week does not generate noise. They are
// starting heuristics, not product claims, and a policy may tune them later.
const (
	DefaultMinimumBytes      = 64 << 20 // ignore series below 64 MiB
	DefaultSuddenGrowthBytes = 1 << 30  // 1 GiB in a single step
	DefaultSuddenGrowthRatio = 0.25     // ...and at least a quarter larger
	DefaultSustainedPeriods  = 3        // consecutive increases before it is a pattern
	DefaultStableRatio       = 0.01     // within 1% counts as unchanged
)

// Options tunes analysis. The zero value is valid and uses the defaults above.
type Options struct {
	Now               time.Time
	MinimumBytes      int64
	SuddenGrowthBytes int64
	SuddenGrowthRatio float64
	SustainedPeriods  int
}

func (o Options) withDefaults() Options {
	if o.Now.IsZero() {
		o.Now = time.Now().UTC()
	}
	if o.MinimumBytes <= 0 {
		o.MinimumBytes = DefaultMinimumBytes
	}
	if o.SuddenGrowthBytes <= 0 {
		o.SuddenGrowthBytes = DefaultSuddenGrowthBytes
	}
	if o.SuddenGrowthRatio <= 0 {
		o.SuddenGrowthRatio = DefaultSuddenGrowthRatio
	}
	if o.SustainedPeriods <= 0 {
		o.SustainedPeriods = DefaultSustainedPeriods
	}
	return o
}

// Point is one observation of a series.
type Point struct {
	ScanID         string    `json:"scanId"`
	At             time.Time `json:"at"`
	AllocatedBytes int64     `json:"allocatedBytes"`
}

// Regression is a change worth surfacing. It carries the evidence that produced
// it so the UI never has to restate the rule.
type Regression struct {
	Kind       string `json:"kind"`
	Severity   string `json:"severity"`
	SeriesKey  string `json:"seriesKey,omitempty"`
	Label      string `json:"label"`
	Detail     string `json:"detail"`
	ScanID     string `json:"scanId,omitempty"`
	DeltaBytes int64  `json:"deltaBytes,omitempty"`
}

// Series is one measured quantity tracked across snapshots.
type Series struct {
	Key    string  `json:"key"`
	Label  string  `json:"label"`
	Points []Point `json:"points,omitempty"`

	FirstBytes    int64   `json:"firstBytes"`
	LatestBytes   int64   `json:"latestBytes"`
	DeltaBytes    int64   `json:"deltaBytes"`
	PercentChange float64 `json:"percentChange"`
	BytesPerDay   float64 `json:"bytesPerDay"`
	Direction     string  `json:"direction"`
	// Uncertain is set when any observation was a lower bound, so the delta is
	// a lower bound too.
	Uncertain   bool         `json:"uncertain,omitempty"`
	Regressions []Regression `json:"regressions,omitempty"`
}

// Report is the analysed history for one scan scope.
type Report struct {
	SnapshotCount int       `json:"snapshotCount"`
	From          time.Time `json:"from,omitzero"`
	To            time.Time `json:"to,omitzero"`
	// ComparedScope is the scope key every included snapshot shares.
	ComparedScope string `json:"comparedScope,omitempty"`
	// SkippedSnapshots counts history recorded under a different set of roots.
	SkippedSnapshots int `json:"skippedSnapshots,omitempty"`

	Total       Series       `json:"total"`
	Series      []Series     `json:"series,omitempty"`
	Regressions []Regression `json:"regressions,omitempty"`
	Notes       []string     `json:"notes,omitempty"`
}

// Analyze builds growth series and regression signals from snapshot history.
//
// Only snapshots sharing the newest scope are compared: a scan of one project
// folder and a scan of the whole home directory are different measurements, and
// subtracting one from the other would invent growth that never happened.
func Analyze(snapshots []Snapshot, options Options) Report {
	options = options.withDefaults()
	if len(snapshots) == 0 {
		return Report{Notes: []string{"no scan history has been recorded yet"}}
	}

	ordered := append([]Snapshot(nil), snapshots...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].CapturedAt.Before(ordered[j].CapturedAt) })

	scope := ordered[len(ordered)-1].ScopeKey
	comparable := make([]Snapshot, 0, len(ordered))
	skipped := 0
	for _, snapshot := range ordered {
		if snapshot.ScopeKey != scope {
			skipped++
			continue
		}
		comparable = append(comparable, snapshot)
	}

	report := Report{
		SnapshotCount:    len(comparable),
		From:             comparable[0].CapturedAt,
		To:               comparable[len(comparable)-1].CapturedAt,
		ComparedScope:    scope,
		SkippedSnapshots: skipped,
	}
	if skipped > 0 {
		report.Notes = append(report.Notes, fmt.Sprintf(
			"%d earlier scan(s) covered a different set of roots and were left out; growth is only comparable within one scope", skipped))
	}
	if len(comparable) < 2 {
		report.Notes = append(report.Notes,
			"a single scan cannot show a trend; scan the same roots again to start a history")
		report.Total = buildSeries(TotalKey, "All scanned storage", comparable, options)
		return report
	}
	report.Notes = append(report.Notes,
		"growth is measured between completed scans, so a series can move because files changed or because a previously unreadable area became readable")

	// The total is built unconditionally so a history missing the total series
	// still yields a well-formed report rather than a zero value the UI would
	// render as an unlabelled, directionless card.
	report.Total = buildSeries(TotalKey, "All scanned storage", comparable, options)

	keys := seriesKeys(comparable)
	report.Series = make([]Series, 0, len(keys))
	for _, key := range keys {
		if key.key == TotalKey {
			continue
		}
		series := buildSeries(key.key, key.label, comparable, options)
		// A series that never reaches the floor is noise, not a trend.
		if series.LatestBytes < options.MinimumBytes && series.FirstBytes < options.MinimumBytes {
			continue
		}
		report.Series = append(report.Series, series)
	}
	sort.Slice(report.Series, func(i, j int) bool {
		if report.Series[i].DeltaBytes != report.Series[j].DeltaBytes {
			return report.Series[i].DeltaBytes > report.Series[j].DeltaBytes
		}
		return report.Series[i].Key < report.Series[j].Key
	})

	report.Regressions = append(report.Regressions, report.Total.Regressions...)
	for _, series := range report.Series {
		report.Regressions = append(report.Regressions, series.Regressions...)
	}
	if advice := adviceRegression(comparable, options); advice != nil {
		report.Regressions = append(report.Regressions, *advice)
	}
	return report
}

type seriesKey struct{ key, label string }

// seriesKeys collects every key observed anywhere in the window, so a series
// that appeared partway through is still tracked.
func seriesKeys(snapshots []Snapshot) []seriesKey {
	labels := map[string]string{}
	for _, snapshot := range snapshots {
		for _, measurement := range snapshot.Measurements {
			if _, known := labels[measurement.Key]; !known || labels[measurement.Key] == "" {
				labels[measurement.Key] = measurement.Label
			}
		}
	}
	keys := make([]seriesKey, 0, len(labels))
	for key, label := range labels {
		keys = append(keys, seriesKey{key: key, label: label})
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].key < keys[j].key })
	return keys
}

func buildSeries(key, label string, snapshots []Snapshot, options Options) Series {
	series := Series{Key: key, Label: label, Direction: DirectionStable}
	for _, snapshot := range snapshots {
		measurement, found := findMeasurement(snapshot, key)
		if !found {
			// A key absent from a snapshot means the ecosystem or class was not
			// present in that scan, which is a real zero rather than a gap.
			series.Points = append(series.Points, Point{ScanID: snapshot.ScanID, At: snapshot.CapturedAt})
			continue
		}
		if measurement.Uncertain {
			series.Uncertain = true
		}
		series.Points = append(series.Points, Point{
			ScanID:         snapshot.ScanID,
			At:             snapshot.CapturedAt,
			AllocatedBytes: measurement.AllocatedBytes,
		})
	}
	if len(series.Points) == 0 {
		return series
	}
	series.FirstBytes = series.Points[0].AllocatedBytes
	series.LatestBytes = series.Points[len(series.Points)-1].AllocatedBytes
	series.DeltaBytes = series.LatestBytes - series.FirstBytes
	if series.FirstBytes > 0 {
		series.PercentChange = round2(float64(series.DeltaBytes) / float64(series.FirstBytes) * 100)
	}
	if elapsed := series.Points[len(series.Points)-1].At.Sub(series.Points[0].At); elapsed > 0 {
		series.BytesPerDay = round2(float64(series.DeltaBytes) / (float64(elapsed) / float64(24*time.Hour)))
	}
	series.Direction = direction(series)
	series.Regressions = detectRegressions(series, options)
	return series
}

func findMeasurement(snapshot Snapshot, key string) (Measurement, bool) {
	for _, measurement := range snapshot.Measurements {
		if measurement.Key == key {
			return measurement, true
		}
	}
	return Measurement{}, false
}

// direction treats small moves as stable so ordinary churn does not read as a
// trend in either direction.
func direction(series Series) string {
	threshold := int64(float64(series.FirstBytes) * DefaultStableRatio)
	if threshold < 1<<20 {
		threshold = 1 << 20
	}
	switch {
	case series.DeltaBytes > threshold:
		return DirectionGrowing
	case series.DeltaBytes < -threshold:
		return DirectionShrinking
	default:
		return DirectionStable
	}
}

func detectRegressions(series Series, options Options) []Regression {
	var found []Regression
	if sudden := detectSuddenGrowth(series, options); sudden != nil {
		found = append(found, *sudden)
	}
	if sustained := detectSustainedGrowth(series, options); sustained != nil {
		found = append(found, *sustained)
	}
	return found
}

// detectSuddenGrowth flags a single step that is both large in absolute terms
// and large relative to what was already there. Both conditions are required:
// an absolute rule alone shouts about every large machine, and a ratio alone
// shouts about every small series.
func detectSuddenGrowth(series Series, options Options) *Regression {
	if len(series.Points) < 2 {
		return nil
	}
	previous := series.Points[len(series.Points)-2]
	latest := series.Points[len(series.Points)-1]
	delta := latest.AllocatedBytes - previous.AllocatedBytes
	if delta < options.SuddenGrowthBytes {
		return nil
	}
	if previous.AllocatedBytes > 0 && float64(delta)/float64(previous.AllocatedBytes) < options.SuddenGrowthRatio {
		return nil
	}
	return &Regression{
		Kind:       RegressionSuddenGrowth,
		Severity:   SeverityWarning,
		SeriesKey:  series.Key,
		Label:      series.Label,
		ScanID:     latest.ScanID,
		DeltaBytes: delta,
		Detail: fmt.Sprintf("%s grew by %s between the last two scans of these roots",
			series.Label, humanBytes(delta)),
	}
}

// detectSustainedGrowth flags a run of consecutive increases. It is a notice
// rather than a warning: steady growth is often just work happening.
func detectSustainedGrowth(series Series, options Options) *Regression {
	if len(series.Points) <= options.SustainedPeriods {
		return nil
	}
	window := series.Points[len(series.Points)-(options.SustainedPeriods+1):]
	total := int64(0)
	for index := 1; index < len(window); index++ {
		step := window[index].AllocatedBytes - window[index-1].AllocatedBytes
		if step <= 0 {
			return nil
		}
		total += step
	}
	if total < options.SuddenGrowthBytes {
		return nil
	}
	return &Regression{
		Kind:       RegressionSustainedGrowth,
		Severity:   SeverityNotice,
		SeriesKey:  series.Key,
		Label:      series.Label,
		ScanID:     window[len(window)-1].ScanID,
		DeltaBytes: total,
		Detail: fmt.Sprintf("%s grew in %d consecutive scans, %s in total",
			series.Label, options.SustainedPeriods, humanBytes(total)),
	}
}

// adviceRegression reports recoverable storage climbing across scans. The bound
// used is the low bound: the upper bound sums overlapping recommendations and
// would overstate the change (see the Phase 3 deduplication task).
func adviceRegression(snapshots []Snapshot, options Options) *Regression {
	if len(snapshots) < 2 {
		return nil
	}
	previous := snapshots[len(snapshots)-2]
	latest := snapshots[len(snapshots)-1]
	delta := latest.SavingsLowBytes - previous.SavingsLowBytes
	if delta < options.SuddenGrowthBytes {
		return nil
	}
	return &Regression{
		Kind:       RegressionAdviceGrowth,
		Severity:   SeverityNotice,
		Label:      "Recoverable storage",
		ScanID:     latest.ScanID,
		DeltaBytes: delta,
		Detail: fmt.Sprintf("the conservative savings estimate rose by %s, from %d recommendation(s) to %d",
			humanBytes(delta), previous.RecommendationCount, latest.RecommendationCount),
	}
}

func round2(value float64) float64 {
	return float64(int64(value*100+0.5)) / 100
}

// humanBytes formats detail strings. It is intentionally coarse: a trend note
// is prose, and the exact figure travels in DeltaBytes.
func humanBytes(value int64) string {
	const unit = 1024
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}
	div, exp := int64(unit), 0
	for n := value / unit; n >= unit && exp < 4; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(value)/float64(div), "KMGTP"[exp])
}

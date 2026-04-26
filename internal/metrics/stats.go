package metrics

import (
	"strings"
	"sync"
	"time"
)

const (
	minuteWindow      = time.Minute
	maxMinuteBuckets  = 180
	maxRequestSamples = 6000
)

type minuteBucket struct {
	MinuteKey        int64 `json:"minuteKey"`
	Requests         int   `json:"requests"`
	Admin            int   `json:"admin"`
	Chat             int   `json:"chat"`
	Models           int   `json:"models"`
	Image            int   `json:"image"`
	Video            int   `json:"video"`
	Upload           int   `json:"upload"`
	Errors           int   `json:"errors"`
	PromptTokens     int   `json:"promptTokens"`
	CompletionTokens int   `json:"completionTokens"`
	TotalTokens      int   `json:"totalTokens"`
}

type ModelUsage struct {
	PromptTokens     int `json:"promptTokens"`
	CompletionTokens int `json:"completionTokens"`
	TotalTokens      int `json:"totalTokens"`
}

type DashboardStats struct {
	mu            sync.Mutex
	startedAt     time.Time
	totals        minuteBucket
	requestEvents []time.Time
	minutes       map[int64]*minuteBucket
	modelUsage    map[string]ModelUsage
}

func NewDashboardStats() *DashboardStats {
	return &DashboardStats{
		startedAt:  time.Now(),
		minutes:    map[int64]*minuteBucket{},
		modelUsage: map[string]ModelUsage{},
	}
}

func minuteKey(ts time.Time) int64 {
	return ts.Unix() / 60
}

func (d *DashboardStats) ensureBucket(ts time.Time) *minuteBucket {
	key := minuteKey(ts)
	bucket, ok := d.minutes[key]
	if !ok {
		bucket = &minuteBucket{MinuteKey: key}
		d.minutes[key] = bucket
	}
	return bucket
}

func (d *DashboardStats) prune(now time.Time) {
	minThreshold := minuteKey(now.Add(-time.Duration(maxMinuteBuckets) * minuteWindow))
	for key := range d.minutes {
		if key < minThreshold {
			delete(d.minutes, key)
		}
	}

	requestThreshold := now.Add(-time.Hour)
	filtered := d.requestEvents[:0]
	for _, item := range d.requestEvents {
		if item.After(requestThreshold) {
			filtered = append(filtered, item)
		}
	}
	d.requestEvents = filtered
	if len(d.requestEvents) > maxRequestSamples {
		d.requestEvents = d.requestEvents[len(d.requestEvents)-maxRequestSamples:]
	}
}

func (d *DashboardStats) RecordRequest(kind string, statusCode int) {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	bucket := d.ensureBucket(now)
	if kind != "admin" {
		d.totals.Requests++
		bucket.Requests++
	}

	switch kind {
	case "models":
		d.totals.Models++
		bucket.Models++
	case "admin":
		d.totals.Admin++
		bucket.Admin++
	case "upload":
		d.totals.Upload++
		bucket.Upload++
	case "image":
		d.totals.Image++
		bucket.Image++
	case "video":
		d.totals.Video++
		bucket.Video++
	default:
		d.totals.Chat++
		bucket.Chat++
	}

	if kind != "admin" && statusCode >= 400 {
		d.totals.Errors++
		bucket.Errors++
	}

	if kind != "admin" {
		d.requestEvents = append(d.requestEvents, now)
	}
	d.prune(now)
}

func (d *DashboardStats) RecordUsage(promptTokens, completionTokens, totalTokens int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	now := time.Now()
	bucket := d.ensureBucket(now)
	d.totals.PromptTokens += promptTokens
	d.totals.CompletionTokens += completionTokens
	d.totals.TotalTokens += totalTokens
	bucket.PromptTokens += promptTokens
	bucket.CompletionTokens += completionTokens
	bucket.TotalTokens += totalTokens
	d.prune(now)
}

func (d *DashboardStats) RecordModelUsage(model string, promptTokens, completionTokens, totalTokens int) {
	d.RecordUsage(promptTokens, completionTokens, totalTokens)

	model = strings.TrimSpace(model)
	if model == "" {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	usage := d.modelUsage[model]
	usage.PromptTokens += promptTokens
	usage.CompletionTokens += completionTokens
	usage.TotalTokens += totalTokens
	d.modelUsage[model] = usage
}

func (d *DashboardStats) ModelUsageSnapshot() map[string]ModelUsage {
	d.mu.Lock()
	defer d.mu.Unlock()

	snapshot := make(map[string]ModelUsage, len(d.modelUsage))
	for model, usage := range d.modelUsage {
		snapshot[model] = usage
	}
	return snapshot
}

func (d *DashboardStats) Snapshot() map[string]any {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	d.prune(now)
	lastMinute := now.Add(-time.Minute)
	rpm := 0
	for _, item := range d.requestEvents {
		if item.After(lastMinute) {
			rpm++
		}
	}

	series := make([]map[string]any, 0, 30)
	requests30 := 0
	admin30 := 0
	tokens30 := 0
	peakRequests := 0
	peakTokens := 0
	currentKey := minuteKey(now)
	for offset := 29; offset >= 0; offset-- {
		key := currentKey - int64(offset)
		bucket, ok := d.minutes[key]
		if !ok {
			bucket = &minuteBucket{MinuteKey: key}
		}
		ts := time.Unix(key*60, 0)
		requests30 += bucket.Requests
		admin30 += bucket.Admin
		tokens30 += bucket.TotalTokens
		if bucket.Requests > peakRequests {
			peakRequests = bucket.Requests
		}
		if bucket.TotalTokens > peakTokens {
			peakTokens = bucket.TotalTokens
		}
		series = append(series, map[string]any{
			"time":             ts.UTC().Format(time.RFC3339),
			"label":            ts.Format("15:04"),
			"requests":         bucket.Requests,
			"admin":            bucket.Admin,
			"chat":             bucket.Chat,
			"models":           bucket.Models,
			"image":            bucket.Image,
			"video":            bucket.Video,
			"upload":           bucket.Upload,
			"errors":           bucket.Errors,
			"promptTokens":     bucket.PromptTokens,
			"completionTokens": bucket.CompletionTokens,
			"totalTokens":      bucket.TotalTokens,
		})
	}

	successRate := 100.0
	if d.totals.Requests > 0 {
		successRate = float64(d.totals.Requests-d.totals.Errors) * 100 / float64(d.totals.Requests)
	}

	return map[string]any{
		"startedAt":        d.startedAt.UTC().Format(time.RFC3339),
		"uptimeSeconds":    int(time.Since(d.startedAt).Seconds()),
		"rpm":              rpm,
		"averageRpm":       float64(requests30) / 30,
		"adminRequests30m": admin30,
		"requests30m":      requests30,
		"tokens30m":        tokens30,
		"peakRequests":     peakRequests,
		"peakTokens":       peakTokens,
		"successRate":      successRate,
		"totals": map[string]int{
			"requests":         d.totals.Requests,
			"admin":            d.totals.Admin,
			"chat":             d.totals.Chat,
			"models":           d.totals.Models,
			"image":            d.totals.Image,
			"video":            d.totals.Video,
			"upload":           d.totals.Upload,
			"errors":           d.totals.Errors,
			"promptTokens":     d.totals.PromptTokens,
			"completionTokens": d.totals.CompletionTokens,
			"totalTokens":      d.totals.TotalTokens,
		},
		"minuteSeries": series,
		"requestMix": []map[string]any{
			{"label": "Chat", "value": d.totals.Chat},
			{"label": "Models", "value": d.totals.Models},
			{"label": "Image", "value": d.totals.Image},
			{"label": "Video", "value": d.totals.Video},
			{"label": "Upload", "value": d.totals.Upload},
			{"label": "Admin", "value": d.totals.Admin},
		},
	}
}

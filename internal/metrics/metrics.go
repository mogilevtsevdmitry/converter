package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all application metrics
type Metrics struct {
	jobsTotal           *prometheus.CounterVec
	jobsActive          prometheus.Gauge
	stageDuration       *prometheus.HistogramVec
	stageFailures       *prometheus.CounterVec
	ffmpegProcesses     prometheus.Gauge
	uploadBytesTotal    prometheus.Counter
	uploadDuration      prometheus.Histogram
	diskFreeBytes       prometheus.Gauge
	queueLag            prometheus.Gauge
}

// New creates a new metrics instance
func New() *Metrics {
	m := &Metrics{
		jobsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "converter_jobs_total",
				Help: "Total number of conversion jobs by status",
			},
			[]string{"status"},
		),
		jobsActive: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "converter_jobs_active",
				Help: "Number of currently active conversion jobs",
			},
		),
		stageDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "converter_stage_duration_seconds",
				Help:    "Duration of each conversion stage in seconds",
				Buckets: prometheus.ExponentialBuckets(1, 2, 15), // 1s to ~9 hours
			},
			[]string{"stage"},
		),
		stageFailures: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "converter_stage_failures_total",
				Help: "Total number of stage failures by stage and error class",
			},
			[]string{"stage", "class"},
		),
		ffmpegProcesses: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "converter_ffmpeg_processes_active",
				Help: "Number of currently running FFmpeg processes",
			},
		),
		uploadBytesTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "converter_upload_bytes_total",
				Help: "Total bytes uploaded to S3",
			},
		),
		uploadDuration: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "converter_upload_duration_seconds",
				Help:    "Duration of upload operations in seconds",
				Buckets: prometheus.ExponentialBuckets(0.1, 2, 12), // 0.1s to ~6 minutes
			},
		),
		diskFreeBytes: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "converter_disk_free_bytes",
				Help: "Free disk space in bytes",
			},
		),
		queueLag: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "converter_queue_lag",
				Help: "Number of jobs waiting in queue",
			},
		),
	}

	return m
}

// IncrementJobsTotal increments the jobs total counter
func (m *Metrics) IncrementJobsTotal(status string) {
	m.jobsTotal.WithLabelValues(status).Inc()
}

// IncrementJobsActive increments the active jobs gauge
func (m *Metrics) IncrementJobsActive() {
	m.jobsActive.Inc()
}

// DecrementJobsActive decrements the active jobs gauge
func (m *Metrics) DecrementJobsActive() {
	m.jobsActive.Dec()
}

// SetJobsActive sets the active jobs gauge
func (m *Metrics) SetJobsActive(count float64) {
	m.jobsActive.Set(count)
}

// RecordStageDuration records the duration of a stage
func (m *Metrics) RecordStageDuration(stage string, seconds float64) {
	m.stageDuration.WithLabelValues(stage).Observe(seconds)
}

// IncrementStageFailures increments the stage failures counter
func (m *Metrics) IncrementStageFailures(stage, class string) {
	m.stageFailures.WithLabelValues(stage, class).Inc()
}

// IncrementFFmpegProcesses increments the FFmpeg processes gauge
func (m *Metrics) IncrementFFmpegProcesses() {
	m.ffmpegProcesses.Inc()
}

// DecrementFFmpegProcesses decrements the FFmpeg processes gauge
func (m *Metrics) DecrementFFmpegProcesses() {
	m.ffmpegProcesses.Dec()
}

// SetFFmpegProcesses sets the FFmpeg processes gauge
func (m *Metrics) SetFFmpegProcesses(count float64) {
	m.ffmpegProcesses.Set(count)
}

// AddUploadBytes adds bytes to the upload total
func (m *Metrics) AddUploadBytes(bytes float64) {
	m.uploadBytesTotal.Add(bytes)
}

// RecordUploadDuration records the duration of an upload
func (m *Metrics) RecordUploadDuration(seconds float64) {
	m.uploadDuration.Observe(seconds)
}

// SetDiskFreeBytes sets the disk free bytes gauge
func (m *Metrics) SetDiskFreeBytes(bytes float64) {
	m.diskFreeBytes.Set(bytes)
}

// SetQueueLag sets the queue lag gauge
func (m *Metrics) SetQueueLag(lag float64) {
	m.queueLag.Set(lag)
}

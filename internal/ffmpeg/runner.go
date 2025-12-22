package ffmpeg

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Progress represents FFmpeg progress
type Progress struct {
	Frame     int64
	FPS       float64
	Bitrate   string
	TotalSize int64
	OutTime   time.Duration
	Speed     float64
	Progress  string
}

// ProgressCallback is called with progress updates
type ProgressCallback func(Progress)

// Runner executes FFmpeg commands
type Runner struct {
	ffmpegPath string
	timeout    time.Duration
}

// NewRunner creates a new runner
func NewRunner(ffmpegPath string, timeout time.Duration) *Runner {
	return &Runner{
		ffmpegPath: ffmpegPath,
		timeout:    timeout,
	}
}

// Run executes an FFmpeg command with progress tracking
func (r *Runner) Run(ctx context.Context, args []string, progressFn ProgressCallback) error {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, r.ffmpegPath, args...)

	// Get stdout for progress
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	// Get stderr for errors
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	// Channel to track last progress update
	progressChan := make(chan Progress, 1)
	done := make(chan struct{})

	// Read progress from stdout
	go func() {
		defer close(done)
		scanner := bufio.NewScanner(stdout)
		progress := Progress{}
		for scanner.Scan() {
			line := scanner.Text()
			if updated := parseProgressLine(line, &progress); updated {
				// Send progress to channel (non-blocking)
				select {
				case progressChan <- progress:
				default:
					// Channel full, skip
				}
				if progressFn != nil {
					progressFn(progress)
				}
			}
		}
	}()

	// Periodic heartbeat ticker - calls progressFn every 30 seconds even if no FFmpeg output
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		lastProgress := Progress{}
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case p := <-progressChan:
				lastProgress = p
			case <-ticker.C:
				// Send periodic heartbeat with last known progress
				if progressFn != nil {
					progressFn(lastProgress)
				}
			}
		}
	}()

	// Collect stderr
	var stderrOutput strings.Builder
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			stderrOutput.WriteString(scanner.Text())
			stderrOutput.WriteString("\n")
		}
	}()

	err = cmd.Wait()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("ffmpeg timed out: %w", err)
		}
		if ctx.Err() == context.Canceled {
			return fmt.Errorf("ffmpeg canceled: %w", err)
		}
		return fmt.Errorf("ffmpeg failed: %w\nstderr: %s", err, stderrOutput.String())
	}

	return nil
}

// RunWithCancel executes an FFmpeg command with cancelation support
func (r *Runner) RunWithCancel(ctx context.Context, args []string, progressFn ProgressCallback) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, r.ffmpegPath, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// Get stdout for progress
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	// Read progress from stdout
	go func() {
		scanner := bufio.NewScanner(stdout)
		progress := Progress{}
		for scanner.Scan() {
			line := scanner.Text()
			if updated := parseProgressLine(line, &progress); updated && progressFn != nil {
				progressFn(progress)
			}
		}
	}()

	return cmd, nil
}

// Stop stops an FFmpeg process gracefully
func (r *Runner) Stop(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}

	// Send SIGTERM first
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		// If SIGTERM fails, force kill
		return cmd.Process.Kill()
	}

	// Wait with timeout
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-time.After(10 * time.Second):
		return cmd.Process.Kill()
	case err := <-done:
		return err
	}
}

var (
	frameRegex    = regexp.MustCompile(`frame=\s*(\d+)`)
	fpsRegex      = regexp.MustCompile(`fps=\s*([\d.]+)`)
	bitrateRegex  = regexp.MustCompile(`bitrate=\s*([^\s]+)`)
	totalSizeRegex = regexp.MustCompile(`total_size=\s*(\d+)`)
	outTimeRegex  = regexp.MustCompile(`out_time_ms=\s*(\d+)`)
	speedRegex    = regexp.MustCompile(`speed=\s*([\d.]+)x`)
	progressRegex = regexp.MustCompile(`progress=\s*(\w+)`)
)

func parseProgressLine(line string, progress *Progress) bool {
	updated := false

	if matches := frameRegex.FindStringSubmatch(line); len(matches) > 1 {
		if v, err := strconv.ParseInt(matches[1], 10, 64); err == nil {
			progress.Frame = v
			updated = true
		}
	}

	if matches := fpsRegex.FindStringSubmatch(line); len(matches) > 1 {
		if v, err := strconv.ParseFloat(matches[1], 64); err == nil {
			progress.FPS = v
			updated = true
		}
	}

	if matches := bitrateRegex.FindStringSubmatch(line); len(matches) > 1 {
		progress.Bitrate = matches[1]
		updated = true
	}

	if matches := totalSizeRegex.FindStringSubmatch(line); len(matches) > 1 {
		if v, err := strconv.ParseInt(matches[1], 10, 64); err == nil {
			progress.TotalSize = v
			updated = true
		}
	}

	if matches := outTimeRegex.FindStringSubmatch(line); len(matches) > 1 {
		if v, err := strconv.ParseInt(matches[1], 10, 64); err == nil {
			progress.OutTime = time.Duration(v) * time.Microsecond
			updated = true
		}
	}

	if matches := speedRegex.FindStringSubmatch(line); len(matches) > 1 {
		if v, err := strconv.ParseFloat(matches[1], 64); err == nil {
			progress.Speed = v
			updated = true
		}
	}

	if matches := progressRegex.FindStringSubmatch(line); len(matches) > 1 {
		progress.Progress = matches[1]
		updated = true
	}

	return updated
}

// CalculateProgress calculates percentage progress
func CalculateProgress(current, total time.Duration) int {
	if total == 0 {
		return 0
	}
	progress := int(float64(current) / float64(total) * 100)
	if progress > 100 {
		progress = 100
	}
	return progress
}

// ValidateOutput validates FFmpeg output file
func ValidateOutput(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("output file not found: %w", err)
	}
	if info.Size() == 0 {
		return fmt.Errorf("output file is empty")
	}
	return nil
}

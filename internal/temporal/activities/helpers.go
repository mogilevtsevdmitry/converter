package activities

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tvoe/converter/internal/ffmpeg"
)

// shiftVTTTimestamps shifts all timestamps in a VTT file by the given duration
func shiftVTTTimestamps(vttPath string, shift time.Duration) error {
	content, err := os.ReadFile(vttPath)
	if err != nil {
		return fmt.Errorf("failed to read VTT file: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	var result []string

	timestampRegex := regexp.MustCompile(`(\d{2}:\d{2}:\d{2}\.\d{3})\s*-->\s*(\d{2}:\d{2}:\d{2}\.\d{3})`)

	for _, line := range lines {
		if matches := timestampRegex.FindStringSubmatch(line); len(matches) == 3 {
			startTime, _ := parseVTTTimestamp(matches[1])
			endTime, _ := parseVTTTimestamp(matches[2])

			newStart := startTime + shift
			newEnd := endTime + shift

			line = fmt.Sprintf("%s --> %s", formatVTTTimestamp(newStart), formatVTTTimestamp(newEnd))
		}
		result = append(result, line)
	}

	return os.WriteFile(vttPath, []byte(strings.Join(result, "\n")), 0644)
}

func parseVTTTimestamp(ts string) (time.Duration, error) {
	parts := strings.Split(ts, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid timestamp format")
	}

	hours, _ := strconv.Atoi(parts[0])
	minutes, _ := strconv.Atoi(parts[1])

	secParts := strings.Split(parts[2], ".")
	seconds, _ := strconv.Atoi(secParts[0])
	millis := 0
	if len(secParts) > 1 {
		millis, _ = strconv.Atoi(secParts[1])
	}

	return time.Duration(hours)*time.Hour +
		time.Duration(minutes)*time.Minute +
		time.Duration(seconds)*time.Second +
		time.Duration(millis)*time.Millisecond, nil
}

func formatVTTTimestamp(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60
	millis := int(d.Milliseconds()) % 1000

	return fmt.Sprintf("%02d:%02d:%02d.%03d", hours, minutes, seconds, millis)
}

// createThumbnailTiles creates thumbnail tiles from individual thumbnails
func createThumbnailTiles(ctx context.Context, thumbsDir string, tileX, tileY int, builder *ffmpeg.CommandBuilder, runner *ffmpeg.Runner) ([]string, error) {
	// Find all thumbnails
	entries, err := os.ReadDir(thumbsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read thumbs directory: %w", err)
	}

	var thumbPaths []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "thumb_") && strings.HasSuffix(entry.Name(), ".jpg") {
			thumbPaths = append(thumbPaths, filepath.Join(thumbsDir, entry.Name()))
		}
	}

	sort.Strings(thumbPaths)

	if len(thumbPaths) == 0 {
		return nil, fmt.Errorf("no thumbnails found")
	}

	// Create tiles
	thumbsPerTile := tileX * tileY
	tileCount := (len(thumbPaths) + thumbsPerTile - 1) / thumbsPerTile

	var tilePaths []string
	for i := 0; i < tileCount; i++ {
		start := i * thumbsPerTile
		end := start + thumbsPerTile
		if end > len(thumbPaths) {
			end = len(thumbPaths)
		}

		// Create concat file for tile
		concatPath := filepath.Join(thumbsDir, fmt.Sprintf("tile_%03d_concat.txt", i))
		concatFile, err := os.Create(concatPath)
		if err != nil {
			continue
		}

		for _, p := range thumbPaths[start:end] {
			fmt.Fprintf(concatFile, "file '%s'\n", p)
		}
		concatFile.Close()

		tilePath := filepath.Join(thumbsDir, fmt.Sprintf("tile_%03d.jpg", i))

		// Use ffmpeg to create tile
		args := []string{
			"-y",
			"-f", "concat",
			"-safe", "0",
			"-i", concatPath,
			"-vf", fmt.Sprintf("tile=%dx%d", tileX, tileY),
			tilePath,
		}

		if err := runner.Run(ctx, args, nil); err != nil {
			os.Remove(concatPath)
			continue
		}

		os.Remove(concatPath)
		tilePaths = append(tilePaths, tilePath)
	}

	// Clean up individual thumbnails
	for _, p := range thumbPaths {
		os.Remove(p)
	}

	return tilePaths, nil
}

// generateThumbnailVTT generates a WebVTT file for thumbnail preview
func generateThumbnailVTT(vttPath string, tilePaths []string, interval float64, width, height, tileX, tileY int) error {
	file, err := os.Create(vttPath)
	if err != nil {
		return fmt.Errorf("failed to create VTT file: %w", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	writer.WriteString("WEBVTT\n\n")

	thumbsPerTile := tileX * tileY
	thumbIndex := 0

	for tileIdx, tilePath := range tilePaths {
		tileName := filepath.Base(tilePath)

		for y := 0; y < tileY; y++ {
			for x := 0; x < tileX; x++ {
				startTime := time.Duration(float64(thumbIndex) * interval * float64(time.Second))
				endTime := time.Duration(float64(thumbIndex+1) * interval * float64(time.Second))

				// Write cue
				fmt.Fprintf(writer, "%s --> %s\n",
					formatVTTTimestamp(startTime),
					formatVTTTimestamp(endTime))
				fmt.Fprintf(writer, "%s#xywh=%d,%d,%d,%d\n\n",
					tileName,
					x*width,
					y*height,
					width,
					height)

				thumbIndex++
				if thumbIndex >= (tileIdx+1)*thumbsPerTile {
					break
				}
			}
			if thumbIndex >= (tileIdx+1)*thumbsPerTile {
				break
			}
		}
	}

	return writer.Flush()
}

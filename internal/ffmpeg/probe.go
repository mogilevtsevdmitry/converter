package ffmpeg

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/tvoe/converter/internal/domain"
)

// Prober extracts metadata from video files
type Prober struct {
	ffprobePath string
}

// NewProber creates a new prober
func NewProber(ffprobePath string) *Prober {
	return &Prober{ffprobePath: ffprobePath}
}

// Probe extracts metadata from a video file
func (p *Prober) Probe(ctx context.Context, inputPath string) (*domain.VideoMetadata, error) {
	args := []string{
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		inputPath,
	}

	cmd := exec.CommandContext(ctx, p.ffprobePath, args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed: %w", err)
	}

	var probeData probeOutput
	if err := json.Unmarshal(output, &probeData); err != nil {
		return nil, fmt.Errorf("failed to parse ffprobe output: %w", err)
	}

	return p.parseProbeOutput(&probeData)
}

type probeOutput struct {
	Format  probeFormat   `json:"format"`
	Streams []probeStream `json:"streams"`
}

type probeFormat struct {
	Filename       string `json:"filename"`
	FormatName     string `json:"format_name"`
	FormatLongName string `json:"format_long_name"`
	Duration       string `json:"duration"`
	Size           string `json:"size"`
	BitRate        string `json:"bit_rate"`
}

type probeStream struct {
	Index          int               `json:"index"`
	CodecName      string            `json:"codec_name"`
	CodecLongName  string            `json:"codec_long_name"`
	CodecType      string            `json:"codec_type"`
	Width          int               `json:"width"`
	Height         int               `json:"height"`
	RFrameRate     string            `json:"r_frame_rate"`
	AvgFrameRate   string            `json:"avg_frame_rate"`
	BitRate        string            `json:"bit_rate"`
	Channels       int               `json:"channels"`
	SampleRate     string            `json:"sample_rate"`
	Tags           map[string]string `json:"tags"`
	Disposition    map[string]int    `json:"disposition"`
}

func (p *Prober) parseProbeOutput(data *probeOutput) (*domain.VideoMetadata, error) {
	meta := &domain.VideoMetadata{}

	// Parse format
	if duration, err := strconv.ParseFloat(data.Format.Duration, 64); err == nil {
		meta.Duration = time.Duration(duration * float64(time.Second))
	}
	if size, err := strconv.ParseInt(data.Format.Size, 10, 64); err == nil {
		meta.FileSize = size
	}
	if bitrate, err := strconv.ParseInt(data.Format.BitRate, 10, 64); err == nil {
		meta.Bitrate = bitrate
	}

	meta.Container = normalizeContainer(data.Format.FormatName)

	// Parse streams
	for _, stream := range data.Streams {
		switch stream.CodecType {
		case "video":
			if meta.VideoCodec == "" {
				meta.VideoCodec = stream.CodecName
				meta.Width = stream.Width
				meta.Height = stream.Height
				meta.FPS = parseFrameRate(stream.RFrameRate)
			}
		case "audio":
			audioTrack := domain.AudioTrackInfo{
				Index:    stream.Index,
				Codec:    stream.CodecName,
				Language: getLanguage(stream.Tags),
				Channels: stream.Channels,
			}
			if sr, err := strconv.Atoi(stream.SampleRate); err == nil {
				audioTrack.SampleRate = sr
			}
			if br, err := strconv.ParseInt(stream.BitRate, 10, 64); err == nil {
				audioTrack.Bitrate = br
			}
			meta.AudioTracks = append(meta.AudioTracks, audioTrack)
			if meta.AudioCodec == "" {
				meta.AudioCodec = stream.CodecName
			}
		case "subtitle":
			subTrack := domain.SubtitleTrackInfo{
				Index:    stream.Index,
				Codec:    stream.CodecName,
				Language: getLanguage(stream.Tags),
				Title:    stream.Tags["title"],
			}
			meta.SubtitleTracks = append(meta.SubtitleTracks, subTrack)
		}
	}

	return meta, nil
}

func parseFrameRate(rate string) float64 {
	parts := strings.Split(rate, "/")
	if len(parts) != 2 {
		return 0
	}
	num, err1 := strconv.ParseFloat(parts[0], 64)
	den, err2 := strconv.ParseFloat(parts[1], 64)
	if err1 != nil || err2 != nil || den == 0 {
		return 0
	}
	return num / den
}

func getLanguage(tags map[string]string) string {
	if lang, ok := tags["language"]; ok {
		return lang
	}
	return "und"
}

func normalizeContainer(format string) string {
	formats := strings.Split(format, ",")
	if len(formats) > 0 {
		f := formats[0]
		switch f {
		case "matroska", "webm":
			return "mkv"
		default:
			return f
		}
	}
	return format
}

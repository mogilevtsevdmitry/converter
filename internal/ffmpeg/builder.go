package ffmpeg

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tvoe/converter/internal/domain"
)

// CommandBuilder builds FFmpeg commands
type CommandBuilder struct {
	ffmpegPath string
	enableGPU  bool
}

// NewCommandBuilder creates a new command builder
func NewCommandBuilder(ffmpegPath string, enableGPU bool) *CommandBuilder {
	return &CommandBuilder{
		ffmpegPath: ffmpegPath,
		enableGPU:  enableGPU,
	}
}

// TranscodeCommand holds transcode command parameters
type TranscodeCommand struct {
	Args       []string
	OutputPath string
}

// BuildTranscodeCommand builds a transcode command for a specific quality
func (b *CommandBuilder) BuildTranscodeCommand(
	inputPath string,
	outputDir string,
	quality domain.Quality,
	metadata *domain.VideoMetadata,
	profile domain.Profile,
) *TranscodeCommand {
	params := quality.Params()
	outputPath := filepath.Join(outputDir, string(quality)+".mp4")

	args := []string{
		"-y",
		"-i", inputPath,
		"-progress", "pipe:1",
		"-stats_period", "1",
	}

	// Video encoding
	if b.enableGPU {
		args = append(args, b.buildGPUVideoArgs(quality, params, metadata, profile)...)
	} else {
		args = append(args, b.buildCPUVideoArgs(quality, params, metadata, profile)...)
	}

	// Audio encoding
	args = append(args, b.buildAudioArgs(metadata)...)

	// Output format
	args = append(args,
		"-movflags", "+faststart",
		outputPath,
	)

	return &TranscodeCommand{
		Args:       args,
		OutputPath: outputPath,
	}
}

func (b *CommandBuilder) buildGPUVideoArgs(quality domain.Quality, params domain.QualityConfig, metadata *domain.VideoMetadata, profile domain.Profile) []string {
	args := []string{
		"-hwaccel", "cuda",
		"-hwaccel_output_format", "cuda",
		"-c:v", "h264_nvenc",
		"-preset", "p4",
		"-tune", "hq",
		"-rc", "vbr",
		"-cq", "23",
	}

	if quality != domain.QualityOrigin {
		args = append(args, "-vf", fmt.Sprintf("scale_cuda=%d:%d:force_original_aspect_ratio=decrease", params.Width, params.Height))
		args = append(args, "-b:v", params.VideoBitrate)
		args = append(args, "-maxrate", params.MaxBitrate)
		args = append(args, "-bufsize", params.BufSize)
	}

	// GOP settings
	gop := profile.Algorithm.GOP
	if gop == 0 {
		gop = 48
	}
	args = append(args, "-g", fmt.Sprintf("%d", gop))

	return args
}

func (b *CommandBuilder) buildCPUVideoArgs(quality domain.Quality, params domain.QualityConfig, metadata *domain.VideoMetadata, profile domain.Profile) []string {
	args := []string{
		"-c:v", "libx264",
		"-preset", "medium",
		"-crf", "23",
		"-profile:v", "high",
		"-level", "4.1",
	}

	if quality != domain.QualityOrigin {
		args = append(args, "-vf", fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2",
			params.Width, params.Height, params.Width, params.Height))
		args = append(args, "-b:v", params.VideoBitrate)
		args = append(args, "-maxrate", params.MaxBitrate)
		args = append(args, "-bufsize", params.BufSize)
	}

	// GOP settings
	gop := profile.Algorithm.GOP
	if gop == 0 {
		gop = 48
	}
	args = append(args, "-g", fmt.Sprintf("%d", gop))
	args = append(args, "-keyint_min", fmt.Sprintf("%d", gop))
	args = append(args, "-sc_threshold", "0")

	return args
}

func (b *CommandBuilder) buildAudioArgs(metadata *domain.VideoMetadata) []string {
	args := []string{
		"-c:a", "aac",
		"-ar", "48000",
		"-ac", "2",
		"-b:a", "192k",
	}

	// Check if downmix is needed
	for _, track := range metadata.AudioTracks {
		if track.Channels > 2 {
			args = append(args, "-af", "aresample=async=1000")
			break
		}
	}

	return args
}

// BuildHLSCommand builds HLS segmentation command
func (b *CommandBuilder) BuildHLSCommand(
	inputPath string,
	outputDir string,
	quality string,
	segmentDuration int,
) *TranscodeCommand {
	playlistPath := filepath.Join(outputDir, quality+".m3u8")
	segmentPath := filepath.Join(outputDir, quality+"_%05d.ts")

	args := []string{
		"-y",
		"-i", inputPath,
		"-c", "copy",
		"-f", "hls",
		"-hls_time", fmt.Sprintf("%d", segmentDuration),
		"-hls_playlist_type", "vod",
		"-hls_segment_filename", segmentPath,
		"-hls_list_size", "0",
		"-progress", "pipe:1",
		playlistPath,
	}

	return &TranscodeCommand{
		Args:       args,
		OutputPath: playlistPath,
	}
}

// BuildSubtitleExtractCommand builds subtitle extraction command
func (b *CommandBuilder) BuildSubtitleExtractCommand(
	inputPath string,
	outputPath string,
	streamIndex int,
) *TranscodeCommand {
	args := []string{
		"-y",
		"-i", inputPath,
		"-map", fmt.Sprintf("0:%d", streamIndex),
		"-c:s", "webvtt",
		outputPath,
	}

	return &TranscodeCommand{
		Args:       args,
		OutputPath: outputPath,
	}
}

// BuildThumbnailCommand builds thumbnail generation command
func (b *CommandBuilder) BuildThumbnailCommand(
	inputPath string,
	outputPattern string,
	interval float64,
	width, height int,
) *TranscodeCommand {
	args := []string{
		"-y",
		"-i", inputPath,
		"-vf", fmt.Sprintf("fps=1/%f,scale=%d:%d", interval, width, height),
		"-vsync", "vfr",
		"-progress", "pipe:1",
		outputPattern,
	}

	return &TranscodeCommand{
		Args:       args,
		OutputPath: outputPattern,
	}
}

// BuildTileCommand builds thumbnail tile command
func (b *CommandBuilder) BuildTileCommand(
	inputPattern string,
	outputPath string,
	tileX, tileY int,
) *TranscodeCommand {
	args := []string{
		"-y",
		"-i", inputPattern,
		"-vf", fmt.Sprintf("tile=%dx%d", tileX, tileY),
		outputPath,
	}

	return &TranscodeCommand{
		Args:       args,
		OutputPath: outputPath,
	}
}

// BuildConcatCommand builds video concatenation command (for intro)
func (b *CommandBuilder) BuildConcatCommand(
	introPath string,
	mainPath string,
	outputPath string,
) *TranscodeCommand {
	// Create filter complex for concat
	filterComplex := "[0:v:0][0:a:0][1:v:0][1:a:0]concat=n=2:v=1:a=1[outv][outa]"

	args := []string{
		"-y",
		"-i", introPath,
		"-i", mainPath,
		"-filter_complex", filterComplex,
		"-map", "[outv]",
		"-map", "[outa]",
		"-c:v", "libx264",
		"-preset", "medium",
		"-crf", "23",
		"-c:a", "aac",
		"-b:a", "192k",
		"-progress", "pipe:1",
		outputPath,
	}

	return &TranscodeCommand{
		Args:       args,
		OutputPath: outputPath,
	}
}

// GenerateMasterPlaylist generates HLS master playlist content
func GenerateMasterPlaylist(qualities []domain.Quality, include4K bool) string {
	var sb strings.Builder
	sb.WriteString("#EXTM3U\n")
	sb.WriteString("#EXT-X-VERSION:3\n\n")

	for _, q := range qualities {
		if q == domain.Quality2160p && !include4K {
			continue
		}

		params := q.Params()
		bandwidth := parseBitrate(params.VideoBitrate) + parseBitrate(params.AudioBitrate)

		if q == domain.QualityOrigin {
			sb.WriteString(fmt.Sprintf("#EXT-X-STREAM-INF:BANDWIDTH=%d,NAME=\"%s\"\n", bandwidth, q))
		} else {
			sb.WriteString(fmt.Sprintf("#EXT-X-STREAM-INF:BANDWIDTH=%d,RESOLUTION=%dx%d,NAME=\"%s\"\n",
				bandwidth, params.Width, params.Height, q))
		}
		sb.WriteString(fmt.Sprintf("%s.m3u8\n\n", q))
	}

	return sb.String()
}

func parseBitrate(bitrate string) int {
	bitrate = strings.TrimSuffix(bitrate, "k")
	bitrate = strings.TrimSuffix(bitrate, "K")
	var value int
	fmt.Sscanf(bitrate, "%d", &value)
	return value * 1000
}

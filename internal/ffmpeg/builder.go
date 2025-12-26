package ffmpeg

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tvoe/converter/internal/config"
	"github.com/tvoe/converter/internal/domain"
)

// CommandBuilder builds FFmpeg commands
type CommandBuilder struct {
	ffmpegPath     string
	enableGPU      bool
	encodingConfig *config.EncodingConfig
}

// NewCommandBuilder creates a new command builder
func NewCommandBuilder(ffmpegPath string, enableGPU bool, encodingConfig *config.EncodingConfig) *CommandBuilder {
	return &CommandBuilder{
		ffmpegPath:     ffmpegPath,
		enableGPU:      enableGPU,
		encodingConfig: encodingConfig,
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
	}

	// Enable GPU decoding when GPU encoding is enabled
	if b.enableGPU {
		args = append(args,
			"-hwaccel", "cuda",
			"-hwaccel_output_format", "cuda",
		)
	}

	args = append(args,
		"-i", inputPath,
		"-progress", "pipe:1",
		"-stats_period", "1",
	)

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
		"-c:v", "h264_nvenc",
		"-preset", "p2",        // Faster preset for better throughput
		"-tune", "hq",
		"-rc", "vbr",
		"-cq", "23",
		"-b_ref_mode", "middle", // Use B-frames as references for better quality
		"-spatial_aq", "1",      // Spatial AQ for better visual quality
		"-temporal_aq", "1",     // Temporal AQ for motion optimization
	}

	if quality != domain.QualityOrigin {
		// Use GPU-accelerated scaling (scale_npp) for better performance
		// Keep frames in CUDA format for nvenc (no format conversion)
		// -2 means auto-calculate height with even number (required for h264)
		args = append(args, "-vf", fmt.Sprintf("scale_npp=w=%d:h=-2:interp_algo=super",
			params.Width))
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
		"-preset", "slower",
		"-crf", "23",
		"-profile:v", "high",
		"-level", "4.1",
		"-threads", "2",
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

// buildH265GPUArgs builds H.265 video encoding arguments for GPU (NVIDIA NVENC)
func (b *CommandBuilder) buildH265GPUArgs(quality domain.Quality, params domain.QualityConfig, metadata *domain.VideoMetadata, profile domain.Profile) []string {
	crf := 26
	if b.encodingConfig != nil && b.encodingConfig.H265CRF > 0 {
		crf = b.encodingConfig.H265CRF
	}

	args := []string{
		"-c:v", "hevc_nvenc",
		"-preset", "p2",        // Faster preset for better throughput
		"-tune", "hq",
		"-rc", "vbr",
		"-cq", fmt.Sprintf("%d", crf),
		"-tag:v", "hvc1",       // Apple compatibility
		"-b_ref_mode", "middle", // Use B-frames as references
		"-spatial_aq", "1",      // Spatial AQ
		"-temporal_aq", "1",     // Temporal AQ
	}

	if quality != domain.QualityOrigin {
		// Adjust bitrate for H.265 efficiency (40% savings)
		videoBitrate := adjustBitrateForCodec(params.VideoBitrate, domain.VideoCodecH265)
		maxBitrate := adjustBitrateForCodec(params.MaxBitrate, domain.VideoCodecH265)
		bufSize := adjustBitrateForCodec(params.BufSize, domain.VideoCodecH265)

		// Use GPU-accelerated scaling (scale_npp) for better performance
		// Keep frames in CUDA format for hevc_nvenc (no format conversion)
		// -2 means auto-calculate height with even number (required for h265)
		args = append(args, "-vf", fmt.Sprintf("scale_npp=w=%d:h=-2:interp_algo=super",
			params.Width))
		args = append(args, "-b:v", videoBitrate)
		args = append(args, "-maxrate", maxBitrate)
		args = append(args, "-bufsize", bufSize)
	}

	// GOP settings
	gop := profile.Algorithm.GOP
	if gop == 0 {
		gop = 48
	}
	args = append(args, "-g", fmt.Sprintf("%d", gop))

	return args
}

// buildH265CPUArgs builds H.265 video encoding arguments for CPU (libx265)
func (b *CommandBuilder) buildH265CPUArgs(quality domain.Quality, params domain.QualityConfig, metadata *domain.VideoMetadata, profile domain.Profile) []string {
	preset := "medium"
	crf := 26
	if b.encodingConfig != nil {
		if b.encodingConfig.H265Preset != "" {
			preset = b.encodingConfig.H265Preset
		}
		if b.encodingConfig.H265CRF > 0 {
			crf = b.encodingConfig.H265CRF
		}
	}

	args := []string{
		"-c:v", "libx265",
		"-preset", preset,
		"-crf", fmt.Sprintf("%d", crf),
		"-tag:v", "hvc1", // Apple compatibility
		"-x265-params", "log-level=error:pools=2",
		"-threads", "2",
	}

	if quality != domain.QualityOrigin {
		// Adjust bitrate for H.265 efficiency (40% savings)
		videoBitrate := adjustBitrateForCodec(params.VideoBitrate, domain.VideoCodecH265)
		maxBitrate := adjustBitrateForCodec(params.MaxBitrate, domain.VideoCodecH265)
		bufSize := adjustBitrateForCodec(params.BufSize, domain.VideoCodecH265)

		args = append(args, "-vf", fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2",
			params.Width, params.Height, params.Width, params.Height))
		args = append(args, "-b:v", videoBitrate)
		args = append(args, "-maxrate", maxBitrate)
		args = append(args, "-bufsize", bufSize)
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

// adjustBitrateForCodec adjusts bitrate based on codec efficiency
func adjustBitrateForCodec(bitrate string, codec domain.VideoCodec) string {
	multiplier := codec.BitrateMultiplier()

	bitrateStr := strings.TrimSuffix(bitrate, "k")
	bitrateStr = strings.TrimSuffix(bitrateStr, "K")

	var value int
	fmt.Sscanf(bitrateStr, "%d", &value)

	adjusted := int(float64(value) * multiplier)
	return fmt.Sprintf("%dk", adjusted)
}

// BuildHLSCommand builds HLS segmentation command
func (b *CommandBuilder) BuildHLSCommand(
	inputPath string,
	outputDir string,
	quality string,
	segmentDuration int,
) *TranscodeCommand {
	return b.BuildHLSCommandWithEncryption(inputPath, outputDir, quality, segmentDuration, nil)
}

// BuildHLSCommandWithEncryption builds HLS segmentation command with optional encryption
func (b *CommandBuilder) BuildHLSCommandWithEncryption(
	inputPath string,
	outputDir string,
	quality string,
	segmentDuration int,
	encryption *EncryptionInfo,
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
	}

	// Add encryption options if provided
	if encryption != nil {
		args = append(args,
			"-hls_key_info_file", encryption.KeyInfoPath,
		)
	}

	args = append(args,
		"-progress", "pipe:1",
		playlistPath,
	)

	return &TranscodeCommand{
		Args:       args,
		OutputPath: playlistPath,
	}
}

// BuildHLSCommandFMP4 builds HLS segmentation command with fMP4 segments
func (b *CommandBuilder) BuildHLSCommandFMP4(
	inputPath string,
	outputDir string,
	quality string,
	segmentDuration int,
) *TranscodeCommand {
	return b.BuildHLSCommandFMP4WithEncryption(inputPath, outputDir, quality, segmentDuration, nil)
}

// BuildHLSCommandFMP4WithEncryption builds HLS command with fMP4 segments and optional encryption
func (b *CommandBuilder) BuildHLSCommandFMP4WithEncryption(
	inputPath string,
	outputDir string,
	quality string,
	segmentDuration int,
	encryption *EncryptionInfo,
) *TranscodeCommand {
	playlistPath := filepath.Join(outputDir, quality+".m3u8")
	initPath := quality + "_init.mp4"
	segmentPath := filepath.Join(outputDir, quality+"_%05d.m4s")

	args := []string{
		"-y",
		"-i", inputPath,
		"-c", "copy",
		"-f", "hls",
		"-hls_time", fmt.Sprintf("%d", segmentDuration),
		"-hls_playlist_type", "vod",
		"-hls_segment_type", "fmp4",
		"-hls_fmp4_init_filename", initPath,
		"-hls_segment_filename", segmentPath,
		"-hls_list_size", "0",
	}

	// Add encryption options if provided
	if encryption != nil {
		args = append(args,
			"-hls_key_info_file", encryption.KeyInfoPath,
		)
	}

	args = append(args,
		"-progress", "pipe:1",
		playlistPath,
	)

	return &TranscodeCommand{
		Args:       args,
		OutputPath: playlistPath,
	}
}

// BuildTranscodeCommandForTier builds transcode command for a specific encoding tier
func (b *CommandBuilder) BuildTranscodeCommandForTier(
	inputPath string,
	outputDir string,
	quality domain.Quality,
	metadata *domain.VideoMetadata,
	profile domain.Profile,
	tier domain.EncodingTier,
) *TranscodeCommand {
	params := quality.Params()
	outputPath := filepath.Join(outputDir, string(quality)+".mp4")

	args := []string{
		"-y",
	}

	// Enable GPU decoding when GPU encoding is enabled
	if b.enableGPU {
		args = append(args,
			"-hwaccel", "cuda",
			"-hwaccel_output_format", "cuda",
		)
	}

	args = append(args,
		"-i", inputPath,
		"-progress", "pipe:1",
		"-stats_period", "1",
	)

	// Video encoding based on tier
	switch tier {
	case domain.TierModern:
		// H.265 encoding
		if b.enableGPU {
			args = append(args, b.buildH265GPUArgs(quality, params, metadata, profile)...)
		} else {
			args = append(args, b.buildH265CPUArgs(quality, params, metadata, profile)...)
		}
	default:
		// Legacy tier - H.264 encoding
		if b.enableGPU {
			args = append(args, b.buildGPUVideoArgs(quality, params, metadata, profile)...)
		} else {
			args = append(args, b.buildCPUVideoArgs(quality, params, metadata, profile)...)
		}
	}

	// Audio encoding (AAC for both tiers)
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

// BuildHLSCommandForTier builds HLS command for a specific tier (TS or fMP4)
func (b *CommandBuilder) BuildHLSCommandForTier(
	inputPath string,
	outputDir string,
	quality string,
	segmentDuration int,
	tier domain.EncodingTier,
	encryption *EncryptionInfo,
) *TranscodeCommand {
	tierConfig := domain.GetTierConfig(tier)

	if tierConfig.Container == domain.ContainerFMP4 {
		return b.BuildHLSCommandFMP4WithEncryption(inputPath, outputDir, quality, segmentDuration, encryption)
	}
	return b.BuildHLSCommandWithEncryption(inputPath, outputDir, quality, segmentDuration, encryption)
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
		"-preset", "slower",
		"-crf", "23",
		"-threads", "2",
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

// GenerateMasterPlaylist generates HLS master playlist content (legacy single-tier)
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

// GenerateMultiCodecMasterPlaylist generates HLS master playlist with multiple codec tiers
// Browsers will automatically select the best compatible stream based on CODECS attribute
func GenerateMultiCodecMasterPlaylist(qualities []domain.Quality, tiers []domain.EncodingTier, include4K bool) string {
	var sb strings.Builder
	sb.WriteString("#EXTM3U\n")
	sb.WriteString("#EXT-X-VERSION:7\n")
	sb.WriteString("#EXT-X-INDEPENDENT-SEGMENTS\n\n")

	for _, tier := range tiers {
		tierConfig := domain.GetTierConfig(tier)
		codecsAttr := fmt.Sprintf("%s,%s", tierConfig.VideoCodecString, tierConfig.AudioCodecString)

		sb.WriteString(fmt.Sprintf("# %s tier (%s/%s)\n", tier, tierConfig.VideoCodec, tierConfig.AudioCodec))

		for _, q := range qualities {
			if q == domain.Quality2160p && !include4K {
				continue
			}

			params := q.Params()

			// Adjust bandwidth for codec efficiency
			videoBandwidth := int(float64(parseBitrate(params.VideoBitrate)) * tierConfig.VideoCodec.BitrateMultiplier())
			audioBandwidth := parseBitrate(params.AudioBitrate)
			totalBandwidth := videoBandwidth + audioBandwidth

			if q == domain.QualityOrigin {
				sb.WriteString(fmt.Sprintf("#EXT-X-STREAM-INF:BANDWIDTH=%d,CODECS=\"%s\",NAME=\"%s-%s\"\n",
					totalBandwidth, codecsAttr, q, tier))
			} else {
				sb.WriteString(fmt.Sprintf("#EXT-X-STREAM-INF:BANDWIDTH=%d,RESOLUTION=%dx%d,CODECS=\"%s\",NAME=\"%s-%s\"\n",
					totalBandwidth, params.Width, params.Height, codecsAttr, q, tier))
			}
			sb.WriteString(fmt.Sprintf("%s/%s.m3u8\n", tier, q))
		}
		sb.WriteString("\n")
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

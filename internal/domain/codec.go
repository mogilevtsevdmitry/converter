package domain

// VideoCodec represents video codec type
type VideoCodec string

const (
	VideoCodecH264 VideoCodec = "h264"
	VideoCodecH265 VideoCodec = "h265"
)

// AudioCodec represents audio codec type
type AudioCodec string

const (
	AudioCodecAAC AudioCodec = "aac"
)

// ContainerFormat represents container format for HLS segments
type ContainerFormat string

const (
	ContainerTS   ContainerFormat = "ts"
	ContainerFMP4 ContainerFormat = "fmp4"
)

// EncodingTier represents encoding tier with codec combination
type EncodingTier string

const (
	TierLegacy EncodingTier = "legacy" // H264/AAC/TS - maximum compatibility
	TierModern EncodingTier = "modern" // H265/AAC/fMP4 - 40% bandwidth savings
)

// TierConfig holds codec configuration for a tier
type TierConfig struct {
	Tier       EncodingTier
	VideoCodec VideoCodec
	AudioCodec AudioCodec
	Container  ContainerFormat
	// Codec strings for HLS playlist (RFC 6381)
	VideoCodecString string // e.g., "avc1.640028", "hvc1.1.6.L120.90"
	AudioCodecString string // e.g., "mp4a.40.2"
}

// GetTierConfig returns codec configuration for tier
func GetTierConfig(tier EncodingTier) TierConfig {
	configs := map[EncodingTier]TierConfig{
		TierLegacy: {
			Tier:             TierLegacy,
			VideoCodec:       VideoCodecH264,
			AudioCodec:       AudioCodecAAC,
			Container:        ContainerTS,
			VideoCodecString: "avc1.640028",
			AudioCodecString: "mp4a.40.2",
		},
		TierModern: {
			Tier:             TierModern,
			VideoCodec:       VideoCodecH265,
			AudioCodec:       AudioCodecAAC,
			Container:        ContainerFMP4,
			VideoCodecString: "hvc1.1.6.L120.90",
			AudioCodecString: "mp4a.40.2",
		},
	}
	return configs[tier]
}

// BitrateMultiplier returns bitrate multiplier for codec efficiency
// H.265 achieves same quality at ~60% of H.264 bitrate
func (v VideoCodec) BitrateMultiplier() float64 {
	switch v {
	case VideoCodecH264:
		return 1.0
	case VideoCodecH265:
		return 0.6 // 40% savings
	default:
		return 1.0
	}
}

// SegmentExtension returns file extension for HLS segments
func (c ContainerFormat) SegmentExtension() string {
	switch c {
	case ContainerTS:
		return ".ts"
	case ContainerFMP4:
		return ".m4s"
	default:
		return ".ts"
	}
}

// NeedsInitSegment returns true if container format requires init segment
func (c ContainerFormat) NeedsInitSegment() bool {
	return c == ContainerFMP4
}

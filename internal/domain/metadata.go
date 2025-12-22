package domain

import "time"

// VideoMetadata holds extracted video metadata
type VideoMetadata struct {
	Duration       time.Duration `json:"duration"`
	Width          int           `json:"width"`
	Height         int           `json:"height"`
	Bitrate        int64         `json:"bitrate"`
	FPS            float64       `json:"fps"`
	VideoCodec     string        `json:"videoCodec"`
	AudioCodec     string        `json:"audioCodec"`
	Container      string        `json:"container"`
	AudioTracks    []AudioTrackInfo    `json:"audioTracks"`
	SubtitleTracks []SubtitleTrackInfo `json:"subtitleTracks"`
	FileSize       int64         `json:"fileSize"`
}

// AudioTrackInfo holds audio track metadata
type AudioTrackInfo struct {
	Index      int    `json:"index"`
	Codec      string `json:"codec"`
	Language   string `json:"language"`
	Channels   int    `json:"channels"`
	SampleRate int    `json:"sampleRate"`
	Bitrate    int64  `json:"bitrate"`
}

// SubtitleTrackInfo holds subtitle track metadata
type SubtitleTrackInfo struct {
	Index    int    `json:"index"`
	Codec    string `json:"codec"`
	Language string `json:"language"`
	Title    string `json:"title"`
}

// SupportedContainers lists supported input containers
var SupportedContainers = map[string]bool{
	"mp4":  true,
	"mkv":  true,
	"mov":  true,
	"webm": true,
	"avi":  true,
}

// SupportedVideoCodecs lists supported input video codecs
var SupportedVideoCodecs = map[string]bool{
	"h264":    true,
	"hevc":    true,
	"h265":    true,
	"vp8":     true,
	"vp9":     true,
	"av1":     true,
	"mpeg4":   true,
	"mpeg2video": true,
}

// SupportedAudioCodecs lists supported input audio codecs
var SupportedAudioCodecs = map[string]bool{
	"aac":     true,
	"mp3":     true,
	"ac3":     true,
	"eac3":    true,
	"opus":    true,
	"vorbis":  true,
	"flac":    true,
	"pcm_s16le": true,
	"pcm_s24le": true,
}

// IsContainerSupported checks if container is supported
func IsContainerSupported(container string) bool {
	return SupportedContainers[container]
}

// IsVideoCodecSupported checks if video codec is supported
func IsVideoCodecSupported(codec string) bool {
	return SupportedVideoCodecs[codec]
}

// IsAudioCodecSupported checks if audio codec is supported
func IsAudioCodecSupported(codec string) bool {
	return SupportedAudioCodecs[codec]
}

// FilterQualitiesForResolution filters qualities based on source resolution
func FilterQualitiesForResolution(qualities []Quality, sourceHeight int) []Quality {
	var filtered []Quality
	for _, q := range qualities {
		params := q.Params()
		// Include quality if source is tall enough or it's origin
		if q == QualityOrigin || sourceHeight >= params.Height {
			filtered = append(filtered, q)
		}
	}
	// If all qualities were filtered out, add origin to ensure at least one output
	if len(filtered) == 0 && len(qualities) > 0 {
		filtered = append(filtered, QualityOrigin)
	}
	return filtered
}

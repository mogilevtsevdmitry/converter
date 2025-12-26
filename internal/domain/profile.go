package domain

// Quality represents video quality preset
type Quality string

const (
	Quality480p   Quality = "480p"
	Quality576p   Quality = "576p"
	Quality720p   Quality = "720p"
	Quality1080p  Quality = "1080p"
	Quality1440p  Quality = "1440p"
	Quality2160p  Quality = "2160p"
	QualityOrigin Quality = "origin"
)

// QualityParams returns encoding parameters for quality
func (q Quality) Params() QualityConfig {
	configs := map[Quality]QualityConfig{
		Quality480p: {
			Width:        854,
			Height:       480,
			VideoBitrate: "1500k",
			MaxBitrate:   "2000k",
			BufSize:      "3000k",
			AudioBitrate: "128k",
		},
		Quality576p: {
			Width:        1024,
			Height:       576,
			VideoBitrate: "2000k",
			MaxBitrate:   "2500k",
			BufSize:      "4000k",
			AudioBitrate: "128k",
		},
		Quality720p: {
			Width:        1280,
			Height:       720,
			VideoBitrate: "3000k",
			MaxBitrate:   "4000k",
			BufSize:      "6000k",
			AudioBitrate: "192k",
		},
		Quality1080p: {
			Width:        1920,
			Height:       1080,
			VideoBitrate: "6000k",
			MaxBitrate:   "8000k",
			BufSize:      "12000k",
			AudioBitrate: "256k",
		},
		Quality1440p: {
			Width:        2560,
			Height:       1440,
			VideoBitrate: "10000k",
			MaxBitrate:   "12000k",
			BufSize:      "20000k",
			AudioBitrate: "256k",
		},
		Quality2160p: {
			Width:        3840,
			Height:       2160,
			VideoBitrate: "15000k",
			MaxBitrate:   "20000k",
			BufSize:      "30000k",
			AudioBitrate: "320k",
		},
	}
	if cfg, ok := configs[q]; ok {
		return cfg
	}
	return QualityConfig{}
}

// QualityConfig holds encoding parameters for a quality level
type QualityConfig struct {
	Width        int
	Height       int
	VideoBitrate string
	MaxBitrate   string
	BufSize      string
	AudioBitrate string
}

// AudioTrack represents an audio track configuration
type AudioTrack struct {
	Index    int    `json:"index"`
	Language string `json:"language"`
}

// SubtitleTrack represents a subtitle track configuration
type SubtitleTrack struct {
	Index    int    `json:"index"`
	Language string `json:"language"`
}

// HLSConfig holds HLS generation parameters
type HLSConfig struct {
	SegmentDurationSec int  `json:"segmentDurationSec"`
	PlaylistType       string `json:"playlistType"`
}

// ThumbnailsConfig holds thumbnail generation parameters
type ThumbnailsConfig struct {
	MaxFrames int `json:"maxFrames"`
	TileX     int `json:"tileX"`
	TileY     int `json:"tileY"`
	Width     int `json:"width"`
	Height    int `json:"height"`
}

// IntroConfig holds intro/watermark configuration
type IntroConfig struct {
	S3Key     string `json:"s3Key"`
	ScaleMode string `json:"scaleMode"`
}

// AlgorithmConfig holds A/V sync parameters
type AlgorithmConfig struct {
	FPS            float64 `json:"fps"`
	GOP            int     `json:"gop"`
	AresampleAsync int     `json:"aresampleAsync"`
}

// Profile represents the conversion profile
type Profile struct {
	Qualities   []Quality       `json:"qualities"`
	AudioTracks []AudioTrack    `json:"audioTracks,omitempty"`
	Subtitles   []SubtitleTrack `json:"subtitles,omitempty"`
	HLS         HLSConfig       `json:"hls"`
	Thumbnails  ThumbnailsConfig `json:"thumbnails"`
	Intro       *IntroConfig     `json:"intro,omitempty"`
	Algorithm   AlgorithmConfig  `json:"algorithm"`
}

// DefaultProfile returns a default conversion profile
func DefaultProfile() Profile {
	return Profile{
		Qualities: []Quality{Quality480p, Quality720p, Quality1080p},
		HLS: HLSConfig{
			SegmentDurationSec: 4,
			PlaylistType:       "vod",
		},
		Thumbnails: ThumbnailsConfig{
			MaxFrames: 200,
			TileX:     5,
			TileY:     5,
			Width:     160,
			Height:    90,
		},
		Algorithm: AlgorithmConfig{
			GOP:            48,
			AresampleAsync: 1000,
		},
	}
}

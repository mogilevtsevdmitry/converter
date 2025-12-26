package ffmpeg

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tvoe/converter/internal/domain"
)

// DASHManifest holds DASH MPD generation parameters
type DASHManifest struct {
	Duration        time.Duration
	SegmentDuration int
	Qualities       []domain.Quality
	TierDir         string // e.g., "modern" for fMP4 segments
	BaseURL         string // optional base URL for segments
}

// GenerateDASHManifest generates DASH MPD manifest for fMP4 segments (CMAF compatible)
// This allows the same fMP4 segments to be used for both HLS and DASH
func GenerateDASHManifest(manifest DASHManifest) string {
	var sb strings.Builder

	// ISO 8601 duration format
	durationISO := formatDuration(manifest.Duration)

	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf(`<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" `+
		`xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" `+
		`xsi:schemaLocation="urn:mpeg:dash:schema:mpd:2011 DASH-MPD.xsd" `+
		`profiles="urn:mpeg:dash:profile:isoff-live:2011,urn:com:dashif:dash264" `+
		`type="static" `+
		`mediaPresentationDuration="%s" `+
		`minBufferTime="PT2S">`, durationISO))
	sb.WriteString("\n")

	// Base URL if provided
	if manifest.BaseURL != "" {
		sb.WriteString(fmt.Sprintf("  <BaseURL>%s</BaseURL>\n", manifest.BaseURL))
	}

	sb.WriteString("  <Period>\n")

	// Video AdaptationSet
	sb.WriteString(`    <AdaptationSet mimeType="video/mp4" segmentAlignment="true" startWithSAP="1">`)
	sb.WriteString("\n")

	// Sort qualities by resolution (descending)
	sortedQualities := make([]domain.Quality, len(manifest.Qualities))
	copy(sortedQualities, manifest.Qualities)
	sort.Slice(sortedQualities, func(i, j int) bool {
		pi := sortedQualities[i].Params()
		pj := sortedQualities[j].Params()
		return pi.Width*pi.Height > pj.Width*pj.Height
	})

	for _, q := range sortedQualities {
		if q == domain.QualityOrigin {
			continue // Skip origin for DASH (no fixed resolution)
		}

		params := q.Params()
		// Apply H.265 bitrate multiplier for modern tier
		videoBitrate := int(float64(parseBitrate(params.VideoBitrate)) * domain.VideoCodecH265.BitrateMultiplier())

		qualityStr := string(q)
		initPath := qualityStr + "_init.mp4"
		mediaTemplate := qualityStr + "_$Number%05d$.m4s"

		if manifest.TierDir != "" {
			initPath = manifest.TierDir + "/" + initPath
			mediaTemplate = manifest.TierDir + "/" + mediaTemplate
		}

		sb.WriteString(fmt.Sprintf(`      <Representation id="%s" bandwidth="%d" width="%d" height="%d" codecs="hvc1.1.6.L120.90" frameRate="24">`,
			qualityStr, videoBitrate, params.Width, params.Height))
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf(`        <SegmentTemplate timescale="1000" duration="%d" initialization="%s" media="%s" startNumber="0"/>`,
			manifest.SegmentDuration*1000, initPath, mediaTemplate))
		sb.WriteString("\n")
		sb.WriteString("      </Representation>\n")
	}

	sb.WriteString("    </AdaptationSet>\n")

	// Audio AdaptationSet
	sb.WriteString(`    <AdaptationSet mimeType="audio/mp4" segmentAlignment="true" startWithSAP="1" lang="und">`)
	sb.WriteString("\n")

	// Use first quality for audio (all have same audio)
	if len(sortedQualities) > 0 {
		firstQuality := sortedQualities[0]
		if firstQuality == domain.QualityOrigin && len(sortedQualities) > 1 {
			firstQuality = sortedQualities[1]
		}

		params := firstQuality.Params()
		audioBitrate := parseBitrate(params.AudioBitrate)
		qualityStr := string(firstQuality)
		initPath := qualityStr + "_init.mp4"
		mediaTemplate := qualityStr + "_$Number%05d$.m4s"

		if manifest.TierDir != "" {
			initPath = manifest.TierDir + "/" + initPath
			mediaTemplate = manifest.TierDir + "/" + mediaTemplate
		}

		sb.WriteString(fmt.Sprintf(`      <Representation id="audio" bandwidth="%d" codecs="mp4a.40.2" audioSamplingRate="48000">`,
			audioBitrate))
		sb.WriteString("\n")
		sb.WriteString(`        <AudioChannelConfiguration schemeIdUri="urn:mpeg:dash:23003:3:audio_channel_configuration:2011" value="2"/>`)
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf(`        <SegmentTemplate timescale="1000" duration="%d" initialization="%s" media="%s" startNumber="0"/>`,
			manifest.SegmentDuration*1000, initPath, mediaTemplate))
		sb.WriteString("\n")
		sb.WriteString("      </Representation>\n")
	}

	sb.WriteString("    </AdaptationSet>\n")
	sb.WriteString("  </Period>\n")
	sb.WriteString("</MPD>\n")

	return sb.String()
}

// GenerateDASHManifestWithSegmentList generates DASH MPD with explicit segment list
// This is more accurate but requires scanning the segment files
func GenerateDASHManifestWithSegmentList(
	hlsDir string,
	tierDir string,
	qualities []domain.Quality,
	duration time.Duration,
	segmentDuration int,
) (string, error) {
	var sb strings.Builder

	durationISO := formatDuration(duration)

	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf(`<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" `+
		`profiles="urn:mpeg:dash:profile:isoff-on-demand:2011" `+
		`type="static" `+
		`mediaPresentationDuration="%s" `+
		`minBufferTime="PT2S">`, durationISO))
	sb.WriteString("\n")

	sb.WriteString("  <Period>\n")

	// Video AdaptationSet
	sb.WriteString(`    <AdaptationSet mimeType="video/mp4" segmentAlignment="true" startWithSAP="1">`)
	sb.WriteString("\n")

	// Sort qualities
	sortedQualities := make([]domain.Quality, len(qualities))
	copy(sortedQualities, qualities)
	sort.Slice(sortedQualities, func(i, j int) bool {
		pi := sortedQualities[i].Params()
		pj := sortedQualities[j].Params()
		return pi.Width*pi.Height > pj.Width*pj.Height
	})

	for _, q := range sortedQualities {
		if q == domain.QualityOrigin {
			continue
		}

		params := q.Params()
		videoBitrate := int(float64(parseBitrate(params.VideoBitrate)) * domain.VideoCodecH265.BitrateMultiplier())
		qualityStr := string(q)

		// Find segment files
		segmentDir := hlsDir
		if tierDir != "" {
			segmentDir = filepath.Join(hlsDir, tierDir)
		}

		segments, err := findSegments(segmentDir, qualityStr)
		if err != nil {
			continue // Skip this quality if segments not found
		}

		initFile := qualityStr + "_init.mp4"
		initPath := initFile
		if tierDir != "" {
			initPath = tierDir + "/" + initFile
		}

		sb.WriteString(fmt.Sprintf(`      <Representation id="%s" bandwidth="%d" width="%d" height="%d" codecs="hvc1.1.6.L120.90">`,
			qualityStr, videoBitrate, params.Width, params.Height))
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf(`        <SegmentBase indexRange="0-0">
          <Initialization sourceURL="%s"/>
        </SegmentBase>`, initPath))
		sb.WriteString("\n")
		sb.WriteString("        <SegmentList>\n")

		for _, seg := range segments {
			segPath := seg
			if tierDir != "" {
				segPath = tierDir + "/" + seg
			}
			sb.WriteString(fmt.Sprintf(`          <SegmentURL media="%s"/>`, segPath))
			sb.WriteString("\n")
		}

		sb.WriteString("        </SegmentList>\n")
		sb.WriteString("      </Representation>\n")
	}

	sb.WriteString("    </AdaptationSet>\n")
	sb.WriteString("  </Period>\n")
	sb.WriteString("</MPD>\n")

	return sb.String(), nil
}

// findSegments finds all .m4s segment files for a quality
func findSegments(dir string, quality string) ([]string, error) {
	pattern := filepath.Join(dir, quality+"_*.m4s")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	// Sort by name to ensure correct order
	sort.Strings(matches)

	// Extract just filenames
	segments := make([]string, len(matches))
	for i, m := range matches {
		segments[i] = filepath.Base(m)
	}

	return segments, nil
}

// formatDuration converts Go duration to ISO 8601 duration format
func formatDuration(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := d.Seconds() - float64(hours*3600) - float64(minutes*60)

	if hours > 0 {
		return fmt.Sprintf("PT%dH%dM%.3fS", hours, minutes, seconds)
	} else if minutes > 0 {
		return fmt.Sprintf("PT%dM%.3fS", minutes, seconds)
	}
	return fmt.Sprintf("PT%.3fS", seconds)
}

// WriteDASHManifest writes DASH MPD to file
func WriteDASHManifest(path string, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

package drm

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/tvoe/converter/internal/config"
	"github.com/tvoe/converter/internal/domain"
)

// Provider represents a DRM provider
type Provider string

const (
	ProviderWidevine  Provider = "widevine"
	ProviderFairPlay  Provider = "fairplay"
	ProviderPlayReady Provider = "playready"
	ProviderAll       Provider = "all"
)

// PackageResult holds the result of DRM packaging
type PackageResult struct {
	MasterPlaylistPath string            // HLS master playlist
	MPDPath            string            // DASH MPD manifest
	OutputDir          string            // Directory with all outputs
	KeyID              string            // Key ID used for encryption
	Keys               map[string]string // Provider-specific keys info
}

// Packager wraps Shaka Packager for DRM content protection
type Packager struct {
	config *config.DRMConfig
	binPath string
}

// NewPackager creates a new DRM packager
func NewPackager(cfg *config.DRMConfig) *Packager {
	return &Packager{
		config:  cfg,
		binPath: cfg.ShakaPackagerPath,
	}
}

// IsAvailable checks if Shaka Packager is installed
func (p *Packager) IsAvailable() bool {
	_, err := exec.LookPath(p.binPath)
	return err == nil
}

// Package creates DRM-protected HLS and DASH streams
func (p *Packager) Package(
	ctx context.Context,
	inputPaths map[domain.Quality]string,
	outputDir string,
	jobID uuid.UUID,
) (*PackageResult, error) {
	if !p.IsAvailable() {
		return nil, fmt.Errorf("shaka packager not found at path: %s", p.binPath)
	}

	// Generate or use configured key
	keyID, key, err := p.getOrGenerateKey(jobID)
	if err != nil {
		return nil, fmt.Errorf("failed to get/generate key: %w", err)
	}

	// Build packager arguments
	args := p.buildPackagerArgs(inputPaths, outputDir, keyID, key)

	// Run packager
	cmd := exec.CommandContext(ctx, p.binPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("packager failed: %w\noutput: %s", err, string(output))
	}

	return &PackageResult{
		MasterPlaylistPath: filepath.Join(outputDir, "master.m3u8"),
		MPDPath:            filepath.Join(outputDir, "manifest.mpd"),
		OutputDir:          outputDir,
		KeyID:              keyID,
		Keys: map[string]string{
			"key_id": keyID,
			"key":    key,
		},
	}, nil
}

// getOrGenerateKey returns configured key or generates a new one
func (p *Packager) getOrGenerateKey(jobID uuid.UUID) (keyID, key string, err error) {
	// Use configured Widevine key if available
	if p.config.WidevineKeyID != "" && p.config.WidevineKey != "" {
		return p.config.WidevineKeyID, p.config.WidevineKey, nil
	}

	// Use configured PlayReady key if available
	if p.config.PlayReadyKeyID != "" && p.config.PlayReadyKey != "" {
		return p.config.PlayReadyKeyID, p.config.PlayReadyKey, nil
	}

	// Generate random key for testing/development
	keyIDBytes := make([]byte, 16)
	if _, err := rand.Read(keyIDBytes); err != nil {
		return "", "", err
	}

	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		return "", "", err
	}

	return hex.EncodeToString(keyIDBytes), hex.EncodeToString(keyBytes), nil
}

// buildPackagerArgs builds command line arguments for Shaka Packager
func (p *Packager) buildPackagerArgs(
	inputPaths map[domain.Quality]string,
	outputDir string,
	keyID, key string,
) []string {
	var args []string

	// Add input streams
	for quality, inputPath := range inputPaths {
		qualityStr := string(quality)

		// Video stream
		videoOutput := filepath.Join(outputDir, fmt.Sprintf("%s_video.mp4", qualityStr))
		args = append(args, fmt.Sprintf(
			"in=%s,stream=video,output=%s,playlist_name=%s_video.m3u8",
			inputPath, videoOutput, qualityStr,
		))

		// Audio stream (only for first quality to avoid duplicates)
		if quality == p.getFirstQuality(inputPaths) {
			audioOutput := filepath.Join(outputDir, "audio.mp4")
			args = append(args, fmt.Sprintf(
				"in=%s,stream=audio,output=%s,playlist_name=audio.m3u8,hls_group_id=audio,hls_name=main",
				inputPath, audioOutput,
			))
		}
	}

	// Output manifests
	args = append(args,
		"--hls_master_playlist_output", filepath.Join(outputDir, "master.m3u8"),
		"--mpd_output", filepath.Join(outputDir, "manifest.mpd"),
	)

	// Segment settings
	args = append(args,
		"--segment_duration", "4",
		"--fragment_duration", "4",
	)

	// DRM protection
	args = append(args, p.buildDRMArgs(keyID, key)...)

	return args
}

// buildDRMArgs builds DRM-specific arguments
func (p *Packager) buildDRMArgs(keyID, key string) []string {
	var args []string

	provider := Provider(strings.ToLower(p.config.Provider))

	switch provider {
	case ProviderWidevine:
		args = p.buildWidevineArgs(keyID, key)
	case ProviderFairPlay:
		args = p.buildFairPlayArgs(keyID, key)
	case ProviderPlayReady:
		args = p.buildPlayReadyArgs(keyID, key)
	case ProviderAll:
		// Use raw key encryption which works with all providers
		args = p.buildMultiDRMArgs(keyID, key)
	default:
		// Default to raw key encryption
		args = p.buildRawKeyArgs(keyID, key)
	}

	return args
}

// buildWidevineArgs builds Widevine-specific arguments
func (p *Packager) buildWidevineArgs(keyID, key string) []string {
	args := []string{
		"--enable_raw_key_encryption",
		"--protection_scheme", "cenc",
		"--keys", fmt.Sprintf("key_id=%s:key=%s", keyID, key),
		"--generate_static_live_mpd",
	}

	// Add PSSH if configured
	if p.config.WidevinePSSH != "" {
		args = append(args, "--pssh", p.config.WidevinePSSH)
	}

	return args
}

// buildFairPlayArgs builds FairPlay-specific arguments
func (p *Packager) buildFairPlayArgs(keyID, key string) []string {
	args := []string{
		"--enable_raw_key_encryption",
		"--protection_scheme", "cbcs", // FairPlay uses CBCS
		"--keys", fmt.Sprintf("key_id=%s:key=%s", keyID, key),
	}

	// FairPlay specific
	if p.config.FairPlayKeyURL != "" {
		args = append(args, "--hls_key_uri", p.config.FairPlayKeyURL)
	}
	if p.config.FairPlayIV != "" {
		args = append(args, "--iv", p.config.FairPlayIV)
	}

	return args
}

// buildPlayReadyArgs builds PlayReady-specific arguments
func (p *Packager) buildPlayReadyArgs(keyID, key string) []string {
	args := []string{
		"--enable_raw_key_encryption",
		"--protection_scheme", "cenc",
		"--keys", fmt.Sprintf("key_id=%s:key=%s", keyID, key),
	}

	// PlayReady License URL
	if p.config.PlayReadyLAURL != "" {
		args = append(args, "--playready_la_url", p.config.PlayReadyLAURL)
	}

	return args
}

// buildMultiDRMArgs builds arguments for multi-DRM support
func (p *Packager) buildMultiDRMArgs(keyID, key string) []string {
	args := []string{
		"--enable_raw_key_encryption",
		"--protection_scheme", "cenc", // Compatible with Widevine and PlayReady
		"--keys", fmt.Sprintf("key_id=%s:key=%s", keyID, key),
		"--generate_static_live_mpd",
	}

	// Add Widevine PSSH if available
	if p.config.WidevinePSSH != "" {
		args = append(args, "--pssh", p.config.WidevinePSSH)
	}

	// Add PlayReady LA URL if available
	if p.config.PlayReadyLAURL != "" {
		args = append(args, "--playready_la_url", p.config.PlayReadyLAURL)
	}

	return args
}

// buildRawKeyArgs builds arguments for simple raw key encryption
func (p *Packager) buildRawKeyArgs(keyID, key string) []string {
	return []string{
		"--enable_raw_key_encryption",
		"--protection_scheme", "cenc",
		"--keys", fmt.Sprintf("key_id=%s:key=%s", keyID, key),
	}
}

// getFirstQuality returns the first quality from the map (for audio extraction)
func (p *Packager) getFirstQuality(inputPaths map[domain.Quality]string) domain.Quality {
	// Prefer higher quality for audio
	priorities := []domain.Quality{
		domain.Quality1080p,
		domain.Quality720p,
		domain.Quality480p,
		domain.Quality2160p,
		domain.QualityOrigin,
	}

	for _, q := range priorities {
		if _, ok := inputPaths[q]; ok {
			return q
		}
	}

	// Return any available
	for q := range inputPaths {
		return q
	}

	return domain.Quality720p
}

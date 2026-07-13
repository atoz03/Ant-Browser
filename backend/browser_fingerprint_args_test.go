package backend

import (
	"slices"
	"testing"
)

func TestNormalizeFingerprintLaunchArgsMigratesMacLocaleAndLegacyFlags(t *testing.T) {
	got := normalizeFingerprintLaunchArgs([]string{
		"--fingerprint=42",
		"--fingerprint-platform=mac",
		"--lang=en-US",
		"--window-size=1440,900",
		"--fingerprint-webgl-vendor=Apple",
		"--fingerprint-webgl-renderer=Apple M3",
		"--fingerprint-canvas-noise=true",
		"--fingerprint-audio-noise=false",
		"--webrtc-ip-handling-policy=disable_non_proxied_udp",
	})

	want := []string{
		"--fingerprint=42",
		"--fingerprint-platform=macos",
		"--lang=en-US",
		"--window-size=1440,900",
		"--disable-non-proxied-udp",
		"--accept-lang=en-US,en",
		"--disable-spoofing=audio",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("normalizeFingerprintLaunchArgs() = %#v, want %#v", got, want)
	}
}

func TestNormalizeFingerprintLaunchArgsPreservesExplicitLocaleAndSpoofing(t *testing.T) {
	got := normalizeFingerprintLaunchArgs([]string{
		"--lang=en-GB",
		"--accept-lang=en-GB,en,en-US",
		"--disable-spoofing=gpu,font",
		"--disable-spoofing=audio,gpu",
	})

	want := []string{
		"--lang=en-GB",
		"--accept-lang=en-GB,en",
		"--disable-spoofing=audio,font,gpu",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("normalizeFingerprintLaunchArgs() = %#v, want %#v", got, want)
	}
}

func TestNormalizeFingerprintLaunchArgsSynchronizesAcceptLanguage(t *testing.T) {
	got := normalizeFingerprintLaunchArgs([]string{
		"--lang=en-US",
		"--accept-lang=zh-CN,zh",
	})
	want := []string{"--lang=en-US", "--accept-lang=en-US,en"}
	if !slices.Equal(got, want) {
		t.Fatalf("normalizeFingerprintLaunchArgs() = %#v, want %#v", got, want)
	}
}

func TestNormalizeFingerprintLaunchArgsDropsInvalidSeedAndMigratesRetiredGPUFlag(t *testing.T) {
	got := normalizeFingerprintLaunchArgs([]string{
		"--fingerprint=4876259865204143997",
		"--lang=en-US",
		"--accept-lang=",
		"--fingerprint-gpu-vendor=Apple",
		"--fingerprint-gpu-renderer=Apple M3",
		"--disable-gpu-fingerprint",
	})

	want := []string{
		"--lang=en-US",
		"--accept-lang=en-US,en",
		"--disable-spoofing=gpu",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("normalizeFingerprintLaunchArgs() = %#v, want %#v", got, want)
	}
}

func TestDeterministicFingerprintSeedIsStablePositiveInt32(t *testing.T) {
	profileID := "af25c07f-1134-4d03-92df-aa34b4b71e9e"
	first := deterministicFingerprintSeed(profileID)
	second := deterministicFingerprintSeed(profileID)
	if first != second || first <= 0 || first > maxFingerprintSeed {
		t.Fatalf("deterministicFingerprintSeed() = %d, second = %d", first, second)
	}
}

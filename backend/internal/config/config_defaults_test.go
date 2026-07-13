package config

import (
	"slices"
	"testing"
)

func TestDefaultFingerprintArgsForDarwinUsesMacOSValue(t *testing.T) {
	want := []string{"--fingerprint-brand=Chrome", "--fingerprint-platform=macos"}
	if got := defaultFingerprintArgsForOS("darwin"); !slices.Equal(got, want) {
		t.Fatalf("defaultFingerprintArgsForOS(darwin) = %#v, want %#v", got, want)
	}
}

func TestNormalizeLegacyFingerprintPlatforms(t *testing.T) {
	got := normalizeLegacyFingerprintPlatforms([]string{"--fingerprint=7", "--fingerprint-platform=mac"})
	want := []string{"--fingerprint=7", "--fingerprint-platform=macos"}
	if !slices.Equal(got, want) {
		t.Fatalf("normalizeLegacyFingerprintPlatforms() = %#v, want %#v", got, want)
	}
}

func TestDefaultWindowConfigForDarwinFitsLaptopDisplays(t *testing.T) {
	got := defaultWindowConfigForOS("darwin")
	want := (WindowConfig{Width: 1280, Height: 820, MinWidth: 1000, MinHeight: 650})
	if got != want {
		t.Fatalf("defaultWindowConfigForOS(darwin) = %#v, want %#v", got, want)
	}
}

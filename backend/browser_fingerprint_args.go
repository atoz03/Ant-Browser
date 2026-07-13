package backend

import (
	"hash/fnv"
	"sort"
	"strconv"
	"strings"
)

const maxFingerprintSeed = int64(1<<31 - 1)

var retiredFingerprintArgPrefixes = []string{
	"--fingerprint-color-depth=",
	"--fingerprint-device-memory=",
	"--fingerprint-webgl-vendor=",
	"--fingerprint-webgl-renderer=",
	"--fingerprint-gpu-vendor=",
	"--fingerprint-gpu-renderer=",
	"--fingerprint-fonts=",
	"--fingerprint-do-not-track=",
	"--fingerprint-media-devices=",
	"--fingerprint-touch-points=",
}

func normalizeFingerprintLaunchArgs(input []string) []string {
	result := make([]string, 0, len(input)+3)
	disabledSpoofing := make(map[string]struct{})
	lang := ""
	acceptLang := ""

	for _, raw := range input {
		arg := strings.TrimSpace(raw)
		if arg == "" {
			continue
		}
		switch {
		case strings.HasPrefix(arg, "--fingerprint="):
			value := strings.TrimSpace(strings.TrimPrefix(arg, "--fingerprint="))
			seed, err := strconv.ParseInt(value, 10, 32)
			if err == nil && seed > 0 && seed <= maxFingerprintSeed {
				result = append(result, "--fingerprint="+value)
			}
		case strings.HasPrefix(arg, "--fingerprint-platform="):
			value := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(arg, "--fingerprint-platform=")))
			if value == "mac" || value == "darwin" || value == "osx" {
				value = "macos"
			}
			result = append(result, "--fingerprint-platform="+value)
		case strings.HasPrefix(arg, "--lang="):
			lang = strings.TrimSpace(strings.TrimPrefix(arg, "--lang="))
			if lang != "" {
				result = append(result, "--lang="+lang)
			}
		case strings.HasPrefix(arg, "--accept-lang="):
			value := strings.TrimSpace(strings.TrimPrefix(arg, "--accept-lang="))
			if value != "" {
				acceptLang = value
			}
		case strings.HasPrefix(arg, "--window-size="):
			result = append(result, arg)
		case strings.HasPrefix(arg, "--disable-spoofing="):
			for _, value := range strings.Split(strings.TrimPrefix(arg, "--disable-spoofing="), ",") {
				if value = strings.TrimSpace(value); value != "" {
					disabledSpoofing[value] = struct{}{}
				}
			}
		case arg == "--disable-non-proxied-udp":
			result = append(result, arg)
		case strings.HasPrefix(arg, "--webrtc-ip-handling-policy="):
			value := strings.TrimSpace(strings.TrimPrefix(arg, "--webrtc-ip-handling-policy="))
			if value == "disable_non_proxied_udp" {
				result = append(result, "--disable-non-proxied-udp")
			} else if value != "" {
				result = append(result, "--force-webrtc-ip-handling-policy="+value)
			}
		case strings.HasPrefix(arg, "--fingerprint-canvas-noise="):
			if strings.EqualFold(strings.TrimPrefix(arg, "--fingerprint-canvas-noise="), "false") {
				disabledSpoofing["canvas"] = struct{}{}
			}
		case strings.HasPrefix(arg, "--fingerprint-audio-noise="):
			if strings.EqualFold(strings.TrimPrefix(arg, "--fingerprint-audio-noise="), "false") {
				disabledSpoofing["audio"] = struct{}{}
			}
		case arg == "--disable-gpu-fingerprint":
			disabledSpoofing["gpu"] = struct{}{}
		case hasAnyPrefix(arg, retiredFingerprintArgPrefixes):
			continue
		default:
			result = append(result, arg)
		}
	}

	if lang != "" {
		result = append(result, "--accept-lang="+preferredAcceptLanguages(lang))
	} else if acceptLang != "" {
		result = append(result, "--accept-lang="+acceptLang)
	}
	if len(disabledSpoofing) > 0 {
		values := make([]string, 0, len(disabledSpoofing))
		for value := range disabledSpoofing {
			values = append(values, value)
		}
		sort.Strings(values)
		result = append(result, "--disable-spoofing="+strings.Join(values, ","))
	}
	return result
}

func deterministicFingerprintSeed(profileID string) int64 {
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(strings.TrimSpace(profileID)))
	seed := int64(hasher.Sum32() & uint32(maxFingerprintSeed))
	if seed == 0 {
		return 1
	}
	return seed
}

func preferredAcceptLanguages(lang string) string {
	normalized := strings.TrimSpace(lang)
	parts := strings.SplitN(normalized, "-", 2)
	if len(parts) == 2 && parts[0] != "" {
		return normalized + "," + strings.ToLower(parts[0])
	}
	return normalized
}

func hasAnyPrefix(value string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

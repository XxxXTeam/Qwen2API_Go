package qwen

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

type requestFingerprint struct {
	UserAgent              string
	SecChUA                string
	SecChUAFullVersion     string
	SecChUAFullVersionList string
	SecChUAPlatform        string
	SecChUAPlatformVersion string
	SecChUAMobile          string
	SecChUAArch            string
	SecChUABitness         string
	AcceptLanguage         string
	Timezone               string
	AcceptEncoding         string
	CacheControl           string
	Pragma                 string
	Priority               string
	DNT                    string
}

var fingerprintTimezones = []string{
	"Fri Apr 24 2026 12:00:00 GMT+0800",
	"Thu Apr 23 2026 21:00:00 GMT+0100",
	"Thu Apr 23 2026 16:00:00 GMT-0400",
	"Fri Apr 24 2026 05:30:00 GMT+0130",
	"Fri Apr 24 2026 13:00:00 GMT+0900",
}

var fingerprintLanguages = []string{
	"zh-CN,zh;q=0.9,en-US;q=0.8,en;q=0.7",
	"en-US,en;q=0.9,zh-CN;q=0.7,zh;q=0.6",
	"zh-CN,zh;q=0.95,en;q=0.8",
	"en-GB,en;q=0.9,en-US;q=0.8,zh-CN;q=0.6",
}

var fingerprintPlatforms = []string{
	`"Windows"`,
	`"macOS"`,
}

var fingerprintPlatformVersions = map[string][]string{
	`"Windows"`: {
		`"10.0.0"`,
		`"13.0.0"`,
		`"14.0.0"`,
	},
	`"macOS"`: {
		`"13.6.0"`,
		`"14.4.0"`,
		`"15.0.0"`,
	},
}

var fingerprintArchitectures = []string{
	`"x86"`,
	`"x86_64"`,
	`"arm"`,
}

var fingerprintBitness = []string{
	`"64"`,
	`"32"`,
}

var fingerprintEncodings = []string{
	"gzip, deflate",
	"deflate, gzip",
	"gzip, deflate, identity",
}

var fingerprintPriorities = []string{
	"u=1, i",
	"u=0, i",
}

func fingerprintForToken(token string) requestFingerprint {
	clean := strings.TrimSpace(token)
	if strings.HasPrefix(strings.ToLower(clean), "bearer ") {
		clean = strings.TrimSpace(clean[7:])
	}
	if clean == "" {
		clean = "anonymous"
	}

	sum := sha256.Sum256([]byte(clean))
	seedA := binary.BigEndian.Uint64(sum[0:8])
	seedB := binary.BigEndian.Uint64(sum[8:16])
	seedC := binary.BigEndian.Uint64(sum[16:24])

	family := "Chrome"
	brand := "Google Chrome"
	if seedA%2 == 1 {
		family = "Edge"
		brand = "Microsoft Edge"
	}

	platformToken := "Windows NT 10.0; Win64; x64"
	platformHint := fingerprintPlatforms[0]
	if seedB%3 == 0 {
		platformToken = "Macintosh; Intel Mac OS X 10_15_7"
		platformHint = fingerprintPlatforms[1]
	}
	platformVersions := fingerprintPlatformVersions[platformHint]
	platformVersion := platformVersions[int(seedA%uint64(len(platformVersions)))]
	arch := fingerprintArchitectures[int(seedB%uint64(len(fingerprintArchitectures)))]
	bitness := fingerprintBitness[int(seedC%uint64(len(fingerprintBitness)))]
	if platformHint == `"macOS"` {
		arch = `"arm"`
		bitness = `"64"`
	}

	major := 140 + int(seedA%8)
	minor := int(seedB % 10)
	build := 7000 + int(seedC%900)
	patch := int((seedA + seedB) % 220)
	fullVersion := fmt.Sprintf("%d.%d.%d.%d", major, minor, build, patch)

	chromiumMajor := major
	userAgent := fmt.Sprintf(
		"Mozilla/5.0 (%s) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%d.0.%d.%d Safari/537.36",
		platformToken,
		major,
		build,
		patch,
	)
	if family == "Edge" {
		userAgent += fmt.Sprintf(" Edg/%d.%d.%d.%d", major, minor, build, patch)
	}

	notABrandVersion := 20 + int(seedC%8)
	secChUA := fmt.Sprintf(`"%s";v="%d", "Chromium";v="%d", "Not A(Brand";v="%d"`, brand, major, chromiumMajor, notABrandVersion)
	secChUAFullVersionList := fmt.Sprintf(`"%s";v="%s", "Chromium";v="%s", "Not A(Brand";v="%d.0.0.0"`, brand, fullVersion, fullVersion, notABrandVersion)

	return requestFingerprint{
		UserAgent:              userAgent,
		SecChUA:                secChUA,
		SecChUAFullVersion:     fullVersion,
		SecChUAFullVersionList: secChUAFullVersionList,
		SecChUAPlatform:        platformHint,
		SecChUAPlatformVersion: platformVersion,
		SecChUAMobile:          "?0",
		SecChUAArch:            arch,
		SecChUABitness:         bitness,
		AcceptLanguage:         fingerprintLanguages[int(seedB%uint64(len(fingerprintLanguages)))],
		Timezone:               fingerprintTimezones[int(seedC%uint64(len(fingerprintTimezones)))],
		AcceptEncoding:         fingerprintEncodings[int(seedA%uint64(len(fingerprintEncodings)))],
		CacheControl:           "no-cache",
		Pragma:                 "no-cache",
		Priority:               fingerprintPriorities[int(seedB%uint64(len(fingerprintPriorities)))],
		DNT:                    fmt.Sprintf("%d", seedC%2),
	}
}

func guestRequestFingerprint() requestFingerprint {
	fullVersion := "147.0.0.0"
	return requestFingerprint{
		UserAgent:              "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/147.0.0.0 Safari/537.36 Edg/147.0.0.0",
		SecChUA:                `"Microsoft Edge";v="147", "Not.A/Brand";v="8", "Chromium";v="147"`,
		SecChUAFullVersion:     fullVersion,
		SecChUAFullVersionList: `"Microsoft Edge";v="147.0.0.0", "Chromium";v="147.0.0.0", "Not.A/Brand";v="8.0.0.0"`,
		SecChUAPlatform:        `"Windows"`,
		SecChUAPlatformVersion: `"10.0.0"`,
		SecChUAMobile:          "?0",
		SecChUAArch:            `"x86"`,
		SecChUABitness:         `"64"`,
		AcceptLanguage:         "zh-CN,zh;q=0.9",
		Timezone:               time.Now().Format("Mon Jan 02 2006 15:04:05 GMT-0700"),
		AcceptEncoding:         "gzip, deflate, br",
		CacheControl:           "no-cache",
		Pragma:                 "no-cache",
		Priority:               "u=1, i",
		DNT:                    "0",
	}
}

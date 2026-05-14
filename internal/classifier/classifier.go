// Package classifier applies local semantic heuristics to scanned files.
package classifier

import (
	"regexp"
	"strings"

	"digital-exhaust-cleaner/internal/metadata"
)

// Label names a semantic file class.
type Label string

const (
	// LabelUnknown is used when no local classifier rule matches.
	LabelUnknown Label = "unknown"
	// LabelOTPScreenshot identifies likely one-time-password screenshots.
	LabelOTPScreenshot Label = "otp_screenshot"
	// LabelPaymentScreenshot identifies likely transaction or receipt screenshots.
	LabelPaymentScreenshot Label = "payment_screenshot"
	// LabelQRScreenshot identifies likely QR-code screenshots.
	LabelQRScreenshot Label = "qr_screenshot"
	// LabelTemporaryScreenshot identifies screenshots likely kept for short-term use.
	LabelTemporaryScreenshot Label = "temporary_screenshot"
	// LabelArchive identifies compressed archives.
	LabelArchive Label = "archive"
	// LabelInstaller identifies downloaded installers.
	LabelInstaller Label = "installer"
)

var (
	otpPattern     = regexp.MustCompile(`(?i)(^|[^a-z0-9])(otp|one[-_ ]?time|verification|2fa|mfa)([^a-z0-9]|$)`)
	paymentPattern = regexp.MustCompile(`(?i)(^|[^a-z0-9])(payment|paid|receipt|invoice|transaction|upi|bank|order)([^a-z0-9]|$)`)
	qrPattern      = regexp.MustCompile(`(?i)(^|[^a-z0-9])(qr|qrcode|scan[-_ ]?code)([^a-z0-9]|$)`)
	screenPattern  = regexp.MustCompile(`(?i)(^|[^a-z0-9])(screenshot|screen shot|img_\d+|capture)([^a-z0-9]|$)`)
)

// Classification explains a local semantic decision.
type Classification struct {
	Path        string
	Label       Label
	Confidence  float64
	Rules       []string
	Explanation string
}

// Engine classifies files without uploading content.
type Engine struct{}

// New returns a local classifier engine.
func New() Engine {
	return Engine{}
}

// Classify returns semantic labels for files with a matched rule.
func (Engine) Classify(files []metadata.File) []Classification {
	results := make([]Classification, 0, len(files)/8)
	for _, file := range files {
		result := classifyFile(file)
		if result.Label != LabelUnknown {
			results = append(results, result)
		}
	}
	return results
}

func classifyFile(file metadata.File) Classification {
	searchText := strings.ToLower(file.Path + " " + file.Name + " " + file.Extension)
	isImage := strings.HasPrefix(file.MIMEType, "image/")
	isScreenshot := isImage && screenPattern.MatchString(searchText)

	switch {
	case isScreenshot && otpPattern.MatchString(searchText):
		return matched(file, LabelOTPScreenshot, 0.86, "screenshot_name_contains_otp_terms", "Likely OTP screenshot based on local filename and image metadata.")
	case isScreenshot && paymentPattern.MatchString(searchText):
		return matched(file, LabelPaymentScreenshot, 0.82, "screenshot_name_contains_payment_terms", "Likely payment screenshot based on local filename and image metadata.")
	case isScreenshot && qrPattern.MatchString(searchText):
		return matched(file, LabelQRScreenshot, 0.8, "screenshot_name_contains_qr_terms", "Likely QR screenshot based on local filename and image metadata.")
	case isScreenshot:
		return matched(file, LabelTemporaryScreenshot, 0.68, "generic_screenshot_name", "Screenshot appears temporary based on local filename and image metadata.")
	case isArchive(file.Extension):
		return matched(file, LabelArchive, 0.72, "archive_extension", "Compressed archive detected by extension.")
	case isInstaller(file.Extension):
		return matched(file, LabelInstaller, 0.74, "installer_extension", "Installer package detected by extension.")
	default:
		return Classification{Path: file.Path, Label: LabelUnknown}
	}
}

func matched(file metadata.File, label Label, confidence float64, rule string, explanation string) Classification {
	return Classification{
		Path:        file.Path,
		Label:       label,
		Confidence:  confidence,
		Rules:       []string{rule},
		Explanation: explanation,
	}
}

func isArchive(extension string) bool {
	switch strings.ToLower(extension) {
	case ".zip", ".rar", ".7z", ".tar", ".gz", ".bz2", ".xz":
		return true
	default:
		return false
	}
}

func isInstaller(extension string) bool {
	switch strings.ToLower(extension) {
	case ".exe", ".msi", ".dmg", ".pkg", ".deb", ".rpm":
		return true
	default:
		return false
	}
}

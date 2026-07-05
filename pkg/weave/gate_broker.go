package weave

import (
	"regexp"
	"strings"
)

type GateKind string

const (
	GateNone         GateKind = "none"
	GateTrust        GateKind = "trust"
	GateBrowserOAuth GateKind = "browser_oauth"
	GateDeviceCode   GateKind = "device_code"
	GateAPIKey       GateKind = "api_key"
	GateHuman        GateKind = "human"
)

type GateVerdict struct {
	Kind      GateKind
	URL       string
	Signature string
}

var (
	httpsURLRE        = regexp.MustCompile(`https://[^\s"'<>]+`)
	browserOAuthURLRE = regexp.MustCompile(`(?i)https://[^\s"'<>]*(oauth|authorize|login|callback)[^\s"'<>]*`)
	deviceCodeRE      = regexp.MustCompile(`(?i)\b([A-Z0-9]{4}[- ]?[A-Z0-9]{4}|[A-Z0-9]{6,9})\b`)
)

// classifyGate classifies the current live PTY tail into the kind of
// interactive gate the worker appears to be blocked on. It is intentionally
// pure so the broker can be tested without a live tool, browser, or PTY.
func classifyGate(tail string) GateVerdict {
	low := strings.ToLower(tail)
	if strings.TrimSpace(low) == "" {
		return GateVerdict{Kind: GateNone}
	}
	if sig, ok := findSignature(low, []string{
		"do you trust", "trust the contents", "trust this directory",
		"trust this folder", "continue? 1", "1. yes", "1) yes",
		"yes, continue", "yes, proceed",
	}); ok && (strings.Contains(low, "trust") || strings.Contains(low, "continue?")) {
		return GateVerdict{Kind: GateTrust, Signature: sig}
	}
	if sig, ok := findSignature(low, []string{
		"no api key", "api key not set", "api key is not set",
		"missing api key", "api key required",
	}); ok {
		return GateVerdict{Kind: GateAPIKey, Signature: sig}
	}
	if sig, ok := deviceGateSignature(low); ok {
		url := firstDeviceURL(tail)
		return GateVerdict{Kind: GateDeviceCode, URL: url, Signature: sig}
	}
	if sig, ok := authLoginGateSignature(low); ok {
		if url := firstBrowserOAuthURL(tail); url != "" {
			return GateVerdict{Kind: GateBrowserOAuth, URL: url, Signature: sig}
		}
		return GateVerdict{Kind: GateHuman, Signature: sig}
	}
	return GateVerdict{Kind: GateNone}
}

func findSignature(low string, signatures []string) (string, bool) {
	for _, sig := range signatures {
		if strings.Contains(low, sig) {
			return sig, true
		}
	}
	return "", false
}

func authLoginGateSignature(low string) (string, bool) {
	signatures := append([]string{}, authGateSignatures...)
	signatures = append(signatures,
		"login to continue",
		"log in to continue",
		"open the following url",
		"open the following link",
		"visit this url",
		"visit the url",
		"complete authentication",
	)
	return findSignature(low, signatures)
}

func deviceGateSignature(low string) (string, bool) {
	deviceWords := []string{"device code", "device login", "verification url", "verification uri", "verify at", "device"}
	codeWords := []string{"enter the code", "use code", "user code", "one-time code", "activation code"}
	deviceSig, hasDevice := findSignature(low, deviceWords)
	codeSig, hasCode := findSignature(low, codeWords)
	if !(hasDevice && hasCode) {
		return "", false
	}
	if firstDeviceURL(low) == "" && !deviceCodeRE.MatchString(low) {
		return "", false
	}
	if codeSig != "" {
		return codeSig, true
	}
	return deviceSig, true
}

func firstBrowserOAuthURL(s string) string {
	return trimURLPunctuation(browserOAuthURLRE.FindString(s))
}

func firstDeviceURL(s string) string {
	for _, u := range httpsURLRE.FindAllString(s, -1) {
		clean := trimURLPunctuation(u)
		low := strings.ToLower(clean)
		if strings.Contains(low, "verify") || strings.Contains(low, "device") || strings.Contains(low, "login") {
			return clean
		}
	}
	return ""
}

func trimURLPunctuation(u string) string {
	return strings.TrimRight(u, ".,);]")
}

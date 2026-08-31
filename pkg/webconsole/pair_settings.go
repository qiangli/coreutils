// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package webconsole

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	"github.com/qiangli/coreutils/pkg/coopauth"
)

// The Settings-page twin of `bashy apps pair`.
//
// WHY IT EXISTS. `bashy apps pair` already mints a QR from the terminal. But the
// operator who is looking at the console in a browser has no terminal in front
// of them, and telling them to open one to reach the phone that is in their
// hand is a worse flow than a toggle. This moves the MINT behind a click while
// leaving everything that makes pairing safe exactly where it was.
//
// WHAT IT DOES NOT DO. It does not invent a second, weaker pairing path. The
// ticket is minted by the same pairStore, is single-use and time-boxed the same
// way, confers the same default scope (board/mb/relay — not the terminal, not
// files), and writes the same audit event. It does not broaden the listener:
// on a console that was not started with --pair it FAILS CLOSED and hands back
// the exact restart command, because a Settings toggle must never pretend it
// opened a LAN port that only `apps serve --bind … --pair` can open. No
// firewall or router change is ever made, and nothing here reaches the public
// internet.

// lanAddrFn and mdnsHostFn are the address probes, indirected so a test can pin
// them and assert deterministic dual-labelled output without depending on the
// host's real interfaces (primaryLANAddr is empty offline) or hostname.
var (
	lanAddrFn  = primaryLANAddr
	mdnsHostFn = mdnsName
)

// pairAddress is one dial-able address for a paired phone. Both addresses a
// mint returns carry the SAME single-use ticket: the phone scans whichever code
// its network can resolve, and the ticket closes on the first redemption
// regardless of which one was used.
type pairAddress struct {
	Kind      string `json:"kind"`       // "mdns" | "lan"
	Label     string `json:"label"`      // human label shown under the code
	Host      string `json:"host"`       // the bare host the URL dials
	AccessURL string `json:"access_url"` // clean root URL used after pairing
	URL       string `json:"url"`        // the versioned redeem URL the QR encodes
	QR        string `json:"qr"`         // PNG data URI of URL, or "" if it could not render
}

// handlePairMint mints a pairing ticket for the Settings page and returns
// scan-ready QR presentation for every address a phone could dial.
func (s *server) handlePairMint(w http.ResponseWriter, r *http.Request) {
	// Rate-limit on the PEER address, exactly like /login and /pair/redeem: not
	// X-Forwarded-For, which is attacker-supplied.
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if s.limiter != nil && !s.limiter.Allow(host) {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{
			"enabled": true,
			"error":   "too many pairing attempts; wait a moment and try again",
		})
		return
	}

	// Minting a ticket is an OPERATOR act. A paired phone must not be able to
	// widen the door it came through, and an anonymous LAN visitor must not mint
	// at all. The gate already refuses a device at /api/pair (it is outside every
	// device scope) — this is the fail-closed check that does not depend on that.
	if !s.isOperator(r) {
		writeJSON(w, http.StatusForbidden, map[string]any{
			"enabled": true,
			"error":   "only the signed-in operator can enable phone access",
		})
		return
	}

	// Fail closed when this console cannot broaden its own listener. The toggle
	// must never PRETEND it opened LAN access: pairing is armed by
	// `bashy apps serve --bind <lan-ip> --pair`, and if that was not asked for,
	// the honest answer is the command that would arm it.
	if s.pairing == nil {
		lan := lanAddrFn()
		hint := "<lan-ip>"
		if lan != "" {
			hint = lan
		}
		writeJSON(w, http.StatusConflict, map[string]any{
			"enabled": false,
			"reason":  "phone access is not armed on this console",
			"detail": "The console must be started on the LAN with pairing on before a phone " +
				"can reach it. No firewall or router change is made for you, and this stays on " +
				"your local network — it is never exposed to the internet.",
			"restart":          "bashy apps serve --bind " + hint + " --pair",
			"lan_hint_guessed": lan == "",
		})
		return
	}

	// Default scope/ttl/window (nil, 0, 0): the same board/mb/relay,
	// four-hour-device, two-minute-window a bare `bashy apps pair` confers.
	t, secret, err := s.pairing.issueTicket(nil, 0, 0)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"enabled": true,
			"error":   "could not mint a pairing ticket: " + err.Error(),
		})
		return
	}

	addrs := s.pairAddresses(secret)
	auditPairEvent("pair.issued", map[string]string{
		"ticket": t.ID, "scope": strings.Join(t.Scope, ","),
		"device_ttl": t.TTL, "expires": t.Expires.Format(time.RFC3339),
		"peer": host, "via": "settings",
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":         true,
		"schema":          "bashy-apps-pair-v1",
		"ticket_id":       t.ID,
		"scope":           t.Scope,
		"device_ttl":      t.TTL,
		"expires":         t.Expires,
		"payload_version": pairQRVersion,
		"encrypted":       false,
		"note": "Plaintext HTTP on your LAN: this keeps your OS password off the wire, it does " +
			"not encrypt the link. Fine at home; not on shared wifi.",
		"addresses": addrs,
	})
}

// pairAddresses builds the labelled codes for the Settings page. Both addresses
// are DERIVED, never guessed silently: the mDNS name is what the OS reports as
// this host's own name, the LAN address is the route this host would take to
// reach the network. Each is labelled so the operator knows which one their
// phone can resolve, and a single working code is enough.
func (s *server) pairAddresses(secret string) []pairAddress {
	var out []pairAddress
	if h := mdnsHostFn(); h != "" {
		out = append(out, s.pairAddress("mdns", "Hostname (.local)", h, secret))
	}
	if ip := lanAddrFn(); ip != "" {
		out = append(out, s.pairAddress("lan", "LAN address", ip, secret))
	}
	return out
}

func (s *server) pairAddress(kind, label, host, secret string) pairAddress {
	u := redeemURL(host, s.port, secret)
	access := "http://" + net.JoinHostPort(host, fmt.Sprint(s.port)) + "/"
	a := pairAddress{Kind: kind, Label: label, Host: host, AccessURL: access, URL: u}
	if data, err := qrPNGDataURI(u); err == nil {
		a.QR = data
	}
	return a
}

// qrPNGDataURI renders a payload as a PNG data URI for an <img src>. What it
// encodes is the VERSIONED redeem URL — the single-use ticket in that URL's
// query is the only credential, and there is never a password or a bare
// unauthenticated address in the code.
func qrPNGDataURI(payload string) (string, error) {
	png, err := qrcode.Encode(payload, qrcode.Medium, 256)
	if err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), nil
}

// isOperator reports whether this request is the console's authenticated
// operator, and NOT a paired device. A device-subject session is refused even
// though its cookie is otherwise valid, so a phone that already paired cannot
// mint further tickets.
func (s *server) isOperator(r *http.Request) bool {
	if s.sessions != nil {
		if c, err := r.Cookie(sessionCookie); err == nil {
			if subject, ok := s.sessions.Validate(c.Value); ok {
				_, _, isDevice := splitDeviceSubject(subject)
				return !isDevice
			}
		}
	}
	if coopauth.ArrivedViaCloud(r) {
		return true
	}
	// The ungated loopback owner (a dev console that requires no login) is the
	// operator by the same rule the gate admits them under.
	return !s.requireLogin && coopauth.IsLoopbackAddr(r.RemoteAddr)
}

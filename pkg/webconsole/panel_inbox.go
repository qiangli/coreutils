// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package webconsole

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/bus"
)

// inboxSchemaVersion identifies the panel's payload. Bump it only for a
// breaking shape change; the page reads it and can then say so.
const inboxSchemaVersion = "bashy-console-inbox-v1"

// inboxDefaultLimit / inboxMaxLimit bound one page of an inbox.
//
// Deliberately generous, for the same reason the board panel's caps are: the
// CLI's small caps exist because a message costs an AGENT tokens at a turn
// boundary, and a human scrolling a browser pays neither the tokens nor the
// turn. Inspecting a backlog is the whole reason this panel exists.
const (
	inboxDefaultLimit = 300
	inboxMaxLimit     = 2000
)

// inboxItem is one message as the page renders it.
type inboxItem struct {
	bus.InboxItem
	// Age is a coarse bucket ("today", "week", "older") so the page can offer a
	// time filter without shipping a date picker or re-deriving buckets from a
	// timestamp whose parse it might disagree with.
	Age string `json:"age"`
}

type inboxFacet struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type inboxFacets struct {
	From     []inboxFacet `json:"from"`
	Topic    []inboxFacet `json:"topic"`
	Room     []inboxFacet `json:"room"`
	Delivery []inboxFacet `json:"delivery"`
}

// inboxRoster is the left-hand list: every name on this host that has an inbox.
type inboxRoster struct {
	SchemaVersion string             `json:"schema_version"`
	Viewer        string             `json:"viewer"`
	Holders       []bus.InboxHolder  `json:"holders"`
	Groups        []inboxRosterGroup `json:"groups"`
}

// inboxRosterGroup is one section of the nav, already ordered.
//
// The ORDER is decided here rather than in the page, because it encodes a
// judgment: the person doing the inspecting comes first, always, even with an
// empty inbox. A roster sorted by unread count would move the human around as
// the fleet talks, and the one row a reader navigates to by muscle memory is
// their own.
type inboxRosterGroup struct {
	Kind    string   `json:"kind"`
	Label   string   `json:"label"`
	Names   []string `json:"names"`
	Unread  int      `json:"unread"`
	Total   int      `json:"total"`
	Waiting int      `json:"waiting"` // holders in this group with unread mail
}

type inboxList struct {
	SchemaVersion string      `json:"schema_version"`
	Viewer        string      `json:"viewer"`
	Name          string      `json:"name"`
	Kind          string      `json:"kind"`
	Cursor        int64       `json:"cursor"`
	Total         int         `json:"total"`
	Unread        int         `json:"unread"`
	Matched       int         `json:"matched"`
	Items         []inboxItem `json:"items"`
	Facets        inboxFacets `json:"facets"`
}

// viewerName resolves who is looking, canonicalized through the fleet catalog
// exactly as the CLI's --as goes through it — so the row the page calls "you"
// is the same identity `bashy inbox` would read as.
func (s *server) viewerName(r *http.Request) string {
	viewer, _ := s.userOf(r)
	if canon, err := bus.BoardIdentity(viewer); err == nil && strings.TrimSpace(canon) != "" {
		return canon
	}
	return viewer
}

// handleInboxRoster answers the left nav.
//
// NOTHING here advances a cursor or materializes a record — see
// bus.InspectInboxes. That is not a nicety: this endpoint is polled, so an
// implementation that folded backlog into pending buffers (which the CLI's own
// read path does, correctly, for its own reader) would drain the whole fleet's
// mail every few seconds while nobody was even looking at a message.
func (s *server) handleInboxRoster(w http.ResponseWriter, r *http.Request) {
	viewer := s.viewerName(r)
	holders, err := bus.InspectInboxes(viewer)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	byKind := map[string][]bus.InboxHolder{}
	for _, h := range holders {
		byKind[h.Kind] = append(byKind[h.Kind], h)
	}

	out := inboxRoster{SchemaVersion: inboxSchemaVersion, Viewer: viewer, Holders: holders}
	for _, g := range []struct{ kind, label string }{
		{bus.InboxKindPerson, "You"},
		{bus.InboxKindAgent, "Agents"},
		{bus.InboxKindRole, "Roles"},
		{bus.InboxKindOther, "Other names"},
	} {
		rows := byKind[g.kind]
		if len(rows) == 0 {
			// Every group but the viewer's may legitimately be empty. Omitting
			// it keeps the nav from rendering headings over nothing.
			continue
		}
		group := inboxRosterGroup{Kind: g.kind, Label: g.label}
		for _, h := range rows {
			group.Names = append(group.Names, h.Name)
			group.Total += h.Total
			group.Unread += h.Unread
			if h.Unread > 0 {
				group.Waiting++
			}
		}
		sort.Strings(group.Names)
		out.Groups = append(out.Groups, group)
	}
	writeJSON(w, http.StatusOK, out)
}

// handleInboxList answers one name's inbox, filtered.
//
// Filters are applied server-side and the FACETS are computed over the whole
// inbox rather than over the filtered slice: a dropdown that lists only what
// the current filter already matched cannot be used to change the filter.
func (s *server) handleInboxList(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "no inbox named"})
		return
	}
	viewer := s.viewerName(r)
	view, err := bus.InspectInbox(name, viewer)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	q := r.URL.Query()
	fFrom := strings.TrimSpace(q.Get("from"))
	fTopic := strings.TrimSpace(q.Get("topic"))
	fRoom := strings.TrimSpace(q.Get("room"))
	fDelivery := strings.TrimSpace(q.Get("delivery"))
	fState := strings.TrimSpace(q.Get("state")) // "" | unread | read
	fAge := strings.TrimSpace(q.Get("age"))     // "" | today | week
	fText := strings.ToLower(strings.TrimSpace(q.Get("q")))

	limit := inboxDefaultLimit
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 {
		limit = min(n, inboxMaxLimit)
	}

	from, topic, room, delivery := map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}
	out := make([]inboxItem, 0, limit)
	matched := 0
	for _, it := range view.Items {
		age := ageBucket(it.TS)
		from[it.From]++
		if t := strings.TrimSpace(it.Topic); t != "" {
			topic[t]++
		}
		if rm := strings.TrimSpace(it.Room); rm != "" {
			room[rm]++
		}
		if d := strings.TrimSpace(it.Delivery); d != "" {
			delivery[d]++
		}

		switch {
		case fFrom != "" && !strings.EqualFold(it.From, fFrom),
			fTopic != "" && !strings.EqualFold(it.Topic, fTopic),
			fRoom != "" && !strings.EqualFold(it.Room, fRoom),
			fDelivery != "" && !strings.EqualFold(it.Delivery, fDelivery),
			fState == "unread" && it.Read,
			fState == "read" && !it.Read,
			// "week" means today OR this week — never "unknown", which is a
			// bucket of its own and must not ride in on a time filter.
			fAge == ageToday && age != ageToday,
			fAge == ageWeek && age != ageToday && age != ageWeek,
			fAge == ageOlder && age != ageOlder,
			fAge == ageUnknown && age != ageUnknown:
			continue
		}
		if fText != "" && !strings.Contains(
			strings.ToLower(it.Body+" "+it.From+" "+it.Topic+" "+it.Room), fText) {
			continue
		}
		matched++
		out = append(out, inboxItem{InboxItem: it, Age: age})
	}

	// Chronological, oldest first — the order the page renders. When the cap
	// bites, keep the NEWEST page: an inbox is read from its tail, and trimming
	// the front would silently hide exactly the mail that still needs an answer.
	if len(out) > limit {
		out = out[len(out)-limit:]
	}

	writeJSON(w, http.StatusOK, inboxList{
		SchemaVersion: inboxSchemaVersion,
		Viewer:        viewer,
		Name:          view.Name,
		Kind:          view.Kind,
		Cursor:        view.Cursor,
		Total:         len(view.Items),
		Unread:        view.Unread,
		Matched:       matched,
		Items:         out,
		Facets: inboxFacets{
			From:     inboxFacetList(from),
			Topic:    inboxFacetList(topic),
			Room:     inboxFacetList(room),
			Delivery: inboxFacetList(delivery),
		},
	})
}

// handleInboxPage serves the inbox page.
//
// Like the terminal and the board, it is served from the LAUNCHER's root rather
// than from under a mount, so its <base href> is the launcher's: it reuses
// app.css and the header chrome verbatim, and still composes when the console
// is reached through outpost's tunnel prefix.
func (s *server) handleInboxPage(w http.ResponseWriter, r *http.Request) {
	s.servePageFile(w, r, "inbox.html")
}

// Coarse age buckets. Three, not a date picker: the questions a person actually
// asks of a fleet's mail are "what arrived today", "what arrived this week" and
// "what is still sitting here from before that".
const (
	ageToday   = "today"
	ageWeek    = "week"
	ageOlder   = "older"
	ageUnknown = "unknown"
)

// ageBucket classifies a record's timestamp.
//
// An unparseable or absent stamp is its OWN bucket rather than being folded
// into "older": a record whose time we cannot read is a different fact from one
// we read as old, and quietly filing it under the oldest bucket would hide it
// behind a filter a reader believes is about time.
func ageBucket(ts string) string {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(ts))
	if err != nil {
		return ageUnknown
	}
	switch age := time.Since(t); {
	case age < 24*time.Hour:
		return ageToday
	case age < 7*24*time.Hour:
		return ageWeek
	default:
		return ageOlder
	}
}

func inboxFacetList(m map[string]int) []inboxFacet {
	out := make([]inboxFacet, 0, len(m))
	for n, c := range m {
		if n == "" {
			continue
		}
		out = append(out, inboxFacet{Name: n, Count: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	return out
}

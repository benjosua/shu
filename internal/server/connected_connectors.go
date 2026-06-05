package server

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/mail"
	"net/smtp"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	imapclient "github.com/emersion/go-imap/client"
)

type connectedResource struct {
	ID        string
	Workspace string
	Kind      string
	Locator   string
	Metadata  map[string]any
	Secrets   map[string]any
}

type syncResult struct {
	Seen     int
	Upserted int
	Actions  int
}

func (a *App) loadConnectedResource(ctx context.Context, id string) (connectedResource, error) {
	var r connectedResource
	var metaBytes []byte
	var secretText string
	err := a.db.QueryRow(ctx, `select r.id::text,r.workspace_id::text,r.kind,r.locator,r.metadata,coalesce(s.ciphertext,'') from resources r left join resource_secrets s on s.resource_id=r.id where r.id=$1`, id).Scan(&r.ID, &r.Workspace, &r.Kind, &r.Locator, &metaBytes, &secretText)
	if err != nil {
		return r, err
	}
	_ = json.Unmarshal(metaBytes, &r.Metadata)
	if r.Metadata == nil {
		r.Metadata = map[string]any{}
	}
	plain, err := a.decryptSecret(secretText)
	if err != nil {
		return r, err
	}
	_ = json.Unmarshal(plain, &r.Secrets)
	if r.Secrets == nil {
		r.Secrets = map[string]any{}
	}
	return r, nil
}

func (a *App) testConnectedResource(ctx context.Context, r connectedResource) error {
	driver, ok := connectedDriverFor(r.Kind)
	if !ok {
		return fmt.Errorf("unsupported connected resource kind %s", r.Kind)
	}
	return driver.Test(ctx, a, r)
}

func (a *App) syncConnectedResource(ctx context.Context, resourceID string) (syncResult, error) {
	r, err := a.loadConnectedResource(ctx, resourceID)
	if err != nil {
		return syncResult{}, err
	}
	driver, ok := connectedDriverFor(r.Kind)
	if !ok {
		return syncResult{}, fmt.Errorf("unsupported connected resource kind %s", r.Kind)
	}
	return driver.Sync(ctx, a, r)
}

func connectIMAP(r connectedResource) (*imapclient.Client, error) {
	host := secretString(r, "imap_host")
	port := secretInt(r, "imap_port", 993)
	user := secretString(r, "imap_user")
	pass := secretString(r, "imap_password")
	if host == "" || user == "" || pass == "" {
		return nil, fmt.Errorf("imap host/user/password required")
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	starttls := secretBool(r, "imap_starttls", port != 993)
	var c *imapclient.Client
	var err error
	if starttls {
		c, err = imapclient.Dial(addr)
		if err == nil {
			err = c.StartTLS(&tls.Config{ServerName: host})
		}
	} else {
		c, err = imapclient.DialTLS(addr, &tls.Config{ServerName: host})
	}
	if err != nil {
		return nil, err
	}
	if err := c.Login(user, pass); err != nil {
		_ = c.Logout()
		return nil, err
	}
	return c, nil
}

func (a *App) syncEmailResource(ctx context.Context, r connectedResource) (syncResult, error) {
	c, err := connectIMAP(r)
	if err != nil {
		return syncResult{}, err
	}
	defer c.Logout()
	folders := stringSlice(metaValue(r.Metadata, "folders"))
	if len(folders) == 0 {
		folders = []string{"INBOX"}
	}
	limit := metaInt(r.Metadata, "sync_limit", 50)
	var out syncResult
	for _, folder := range folders {
		if _, err := c.Select(folder, true); err != nil {
			log.Printf("connected email select folder=%s resource=%s: %v", folder, r.ID, err)
			continue
		}
		criteria := imap.NewSearchCriteria()
		if days := metaInt(r.Metadata, "lookback_days", 14); days > 0 {
			criteria.Since = time.Now().Add(-time.Duration(days) * 24 * time.Hour)
		}
		uids, err := c.UidSearch(criteria)
		if err != nil {
			return out, err
		}
		sort.Slice(uids, func(i, j int) bool { return uids[i] > uids[j] })
		if len(uids) > limit {
			uids = uids[:limit]
		}
		if len(uids) == 0 {
			continue
		}
		seq := new(imap.SeqSet)
		seq.AddNum(uids...)
		section := &imap.BodySectionName{}
		items := []imap.FetchItem{imap.FetchEnvelope, imap.FetchFlags, imap.FetchInternalDate, section.FetchItem()}
		msgs := make(chan *imap.Message, len(uids))
		done := make(chan error, 1)
		go func() { done <- c.UidFetch(seq, items, msgs) }()
		for msg := range msgs {
			if msg == nil {
				continue
			}
			out.Seen++
			body := ""
			if br := msg.GetBody(section); br != nil {
				body = readMailBody(br)
			}
			title := ""
			from := ""
			messageID := ""
			occurred := msg.InternalDate
			if msg.Envelope != nil {
				title = msg.Envelope.Subject
				messageID = msg.Envelope.MessageId
				if !msg.Envelope.Date.IsZero() {
					occurred = msg.Envelope.Date
				}
				if len(msg.Envelope.From) > 0 {
					from = msg.Envelope.From[0].Address()
				}
			}
			extID := fmt.Sprintf("%s:%d", folder, msg.Uid)
			state := map[string]any{"uid": msg.Uid, "folder": folder, "from": from, "message_id": messageID, "flags": msg.Flags}
			changed, err := a.upsertItem(ctx, r.Workspace, r.ID, "email.message", extID, title, body, state, []string{}, nil, nil, ptrTime(occurred))
			if err != nil {
				return out, err
			}
			if changed {
				out.Upserted++
				_ = a.handleExternalItemEvent(ctx, r.Workspace, r.ID, "email.message", extID, title, body, state)
			}
		}
		if err := <-done; err != nil {
			return out, err
		}
	}
	return out, nil
}

func readMailBody(r io.Reader) string {
	b, _ := io.ReadAll(io.LimitReader(r, 512*1024))
	msg, err := mail.ReadMessage(bytes.NewReader(b))
	if err != nil {
		return strings.TrimSpace(string(b[:min(len(b), 12000)]))
	}
	body, _ := io.ReadAll(io.LimitReader(msg.Body, 12000))
	return strings.TrimSpace(string(body))
}

func testSMTP(r connectedResource) error {
	host := secretString(r, "smtp_host")
	port := secretInt(r, "smtp_port", 465)
	user := secretString(r, "smtp_user")
	pass := secretString(r, "smtp_password")
	sec := strings.ToLower(secretString(r, "smtp_security"))
	addr := fmt.Sprintf("%s:%d", host, port)
	if host == "" {
		return fmt.Errorf("smtp_host required")
	}
	if sec == "" && port == 465 {
		sec = "ssl"
	}
	if sec == "ssl" {
		conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host})
		if err != nil {
			return err
		}
		defer conn.Close()
		cl, err := smtp.NewClient(conn, host)
		if err != nil {
			return err
		}
		defer cl.Quit()
		if user != "" && pass != "" {
			return cl.Auth(smtp.PlainAuth("", user, pass, host))
		}
		return nil
	}
	cl, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer cl.Quit()
	if sec == "starttls" {
		if err := cl.StartTLS(&tls.Config{ServerName: host}); err != nil {
			return err
		}
	}
	if user != "" && pass != "" {
		return cl.Auth(smtp.PlainAuth("", user, pass, host))
	}
	return nil
}

func (a *App) executeEmailAction(ctx context.Context, r connectedResource, action string, input map[string]any) (map[string]any, error) {
	switch action {
	case "email.send", "email.reply":
		return a.smtpSend(r, input)
	case "email.move", "email.mark_read":
		return a.imapMutate(r, action, input)
	default:
		return nil, fmt.Errorf("unsupported email action %s", action)
	}
}

func (a *App) smtpSend(r connectedResource, input map[string]any) (map[string]any, error) {
	host := secretString(r, "smtp_host")
	port := secretInt(r, "smtp_port", 465)
	user := secretString(r, "smtp_user")
	pass := secretString(r, "smtp_password")
	from := stringAny(input["from"])
	if from == "" {
		from = metaString(r.Metadata, "from_address")
	}
	if from == "" {
		from = user
	}
	to := stringSlice(input["to"])
	if len(to) == 0 {
		return nil, fmt.Errorf("to required")
	}
	subject := stringAny(input["subject"])
	body := stringAny(input["body"])
	msg := buildMailMessage(from, to, subject, body)
	sec := strings.ToLower(secretString(r, "smtp_security"))
	if sec == "" && port == 465 {
		sec = "ssl"
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	if sec == "ssl" {
		conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host})
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		cl, err := smtp.NewClient(conn, host)
		if err != nil {
			return nil, err
		}
		defer cl.Quit()
		if user != "" && pass != "" {
			if err := cl.Auth(smtp.PlainAuth("", user, pass, host)); err != nil {
				return nil, err
			}
		}
		if err := cl.Mail(from); err != nil {
			return nil, err
		}
		for _, rcpt := range to {
			if err := cl.Rcpt(rcpt); err != nil {
				return nil, err
			}
		}
		w, err := cl.Data()
		if err != nil {
			return nil, err
		}
		_, err = w.Write([]byte(msg))
		if cerr := w.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return nil, err
		}
		return map[string]any{"sent": true, "recipients": to}, nil
	}
	auth := smtp.Auth(nil)
	if user != "" && pass != "" {
		auth = smtp.PlainAuth("", user, pass, host)
	}
	if err := smtp.SendMail(addr, auth, from, to, []byte(msg)); err != nil {
		return nil, err
	}
	return map[string]any{"sent": true, "recipients": to}, nil
}

func (a *App) imapMutate(r connectedResource, action string, input map[string]any) (map[string]any, error) {
	c, err := connectIMAP(r)
	if err != nil {
		return nil, err
	}
	defer c.Logout()
	folder := stringAny(input["folder"])
	if folder == "" {
		folder = "INBOX"
	}
	uid := uint32(toInt64(input["uid"]))
	if uid == 0 {
		return nil, fmt.Errorf("uid required")
	}
	if _, err := c.Select(folder, false); err != nil {
		return nil, err
	}
	seq := new(imap.SeqSet)
	seq.AddNum(uid)
	if action == "email.mark_read" {
		seen := true
		if v, ok := input["seen"]; ok {
			seen = boolAny(v)
		}
		var op imap.FlagsOp = imap.AddFlags
		if !seen {
			op = imap.RemoveFlags
		}
		if err := c.UidStore(seq, imap.FormatFlagsOp(op, true), []interface{}{imap.SeenFlag}, nil); err != nil {
			return nil, err
		}
		return map[string]any{"marked_read": seen}, nil
	}
	dest := stringAny(input["dest"])
	if dest == "" {
		return nil, fmt.Errorf("dest required")
	}
	if err := c.UidMove(seq, dest); err != nil {
		return nil, err
	}
	return map[string]any{"moved_to": dest}, nil
}

func buildMailMessage(from string, to []string, subject, body string) string {
	return fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s", from, strings.Join(to, ", "), mime.BEncoding.Encode("utf-8", subject), time.Now().Format(time.RFC1123Z), body)
}

func caldavPropfind(r connectedResource) error {
	url := secretString(r, "caldav_url")
	if url == "" {
		url = r.Locator
	}
	if url == "" {
		return fmt.Errorf("caldav_url required")
	}
	req, err := http.NewRequest("PROPFIND", url, strings.NewReader(`<?xml version="1.0"?><d:propfind xmlns:d="DAV:"><d:prop><d:resourcetype/></d:prop></d:propfind>`))
	if err != nil {
		return err
	}
	req.Header.Set("Depth", "0")
	req.Header.Set("Content-Type", "application/xml")
	user, pass := secretString(r, "caldav_user"), secretString(r, "caldav_password")
	if user != "" || pass != "" {
		req.SetBasicAuth(user, pass)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("caldav HTTP %d", resp.StatusCode)
	}
	return nil
}

func (a *App) syncCalDAVResource(ctx context.Context, r connectedResource) (syncResult, error) {
	url := secretString(r, "caldav_url")
	if url == "" {
		url = r.Locator
	}
	if url == "" {
		return syncResult{}, fmt.Errorf("caldav_url required")
	}
	start := time.Now().Add(-time.Duration(metaInt(r.Metadata, "lookback_days", 90)) * 24 * time.Hour).UTC().Format("20060102T150405Z")
	end := time.Now().Add(time.Duration(metaInt(r.Metadata, "lookahead_days", 365)) * 24 * time.Hour).UTC().Format("20060102T150405Z")
	body := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8" ?><C:calendar-query xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav"><D:prop><D:getetag/><C:calendar-data/></D:prop><C:filter><C:comp-filter name="VCALENDAR"><C:comp-filter name="VEVENT"><C:time-range start="%s" end="%s"/></C:comp-filter></C:comp-filter></C:filter></C:calendar-query>`, start, end)
	req, err := http.NewRequestWithContext(ctx, "REPORT", url, strings.NewReader(body))
	if err != nil {
		return syncResult{}, err
	}
	req.Header.Set("Depth", "1")
	req.Header.Set("Content-Type", "application/xml")
	if u, p := secretString(r, "caldav_user"), secretString(r, "caldav_password"); u != "" || p != "" {
		req.SetBasicAuth(u, p)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return syncResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return syncResult{}, fmt.Errorf("caldav REPORT HTTP %d", resp.StatusCode)
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
	icals := extractCalendarData(string(b))
	var out syncResult
	for _, ical := range icals {
		for _, ev := range parseVEVENTs(ical) {
			out.Seen++
			state := map[string]any{"uid": ev.UID, "location": ev.Location, "description": ev.Description, "rrule": ev.RRule, "all_day": ev.AllDay}
			changed, err := a.upsertItem(ctx, r.Workspace, r.ID, "calendar.event", ev.UID, ev.Summary, ev.Description, state, []string{}, ev.Start, ev.End, ev.Start)
			if err != nil {
				return out, err
			}
			if changed {
				out.Upserted++
				_ = a.handleExternalItemEvent(ctx, r.Workspace, r.ID, "calendar.event", ev.UID, ev.Summary, ev.Description, state)
			}
		}
	}
	return out, nil
}

type vevent struct {
	UID, Summary, Description, Location, RRule string
	Start, End                                 *time.Time
	AllDay                                     bool
}

func extractCalendarData(s string) []string {
	re := regexp.MustCompile(`(?is)<(?:[a-z0-9]+:)?calendar-data[^>]*>(.*?)</(?:[a-z0-9]+:)?calendar-data>`)
	ms := re.FindAllStringSubmatch(s, -1)
	out := []string{}
	for _, m := range ms {
		out = append(out, htmlUnescape(m[1]))
	}
	if len(out) == 0 && strings.Contains(s, "BEGIN:VEVENT") {
		out = append(out, s)
	}
	return out
}

func parseVEVENTs(s string) []vevent {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	re := regexp.MustCompile(`(?s)BEGIN:VEVENT\n(.*?)\nEND:VEVENT`)
	ms := re.FindAllStringSubmatch(s, -1)
	out := []vevent{}
	for _, m := range ms {
		props := icsProps(m[1])
		uid := firstProp(props, "UID")
		if uid == "" {
			continue
		}
		st, allDay := parseICSTime(firstProp(props, "DTSTART"))
		en, _ := parseICSTime(firstProp(props, "DTEND"))
		out = append(out, vevent{UID: uid, Summary: firstProp(props, "SUMMARY"), Description: firstProp(props, "DESCRIPTION"), Location: firstProp(props, "LOCATION"), RRule: firstProp(props, "RRULE"), Start: st, End: en, AllDay: allDay})
	}
	return out
}

func icsProps(s string) map[string][]string {
	lines := unfoldICS(s)
	m := map[string][]string{}
	for _, line := range lines {
		if i := strings.Index(line, ":"); i >= 0 {
			name := strings.ToUpper(strings.Split(line[:i], ";")[0])
			m[name] = append(m[name], icsUnescape(line[i+1:]))
		}
	}
	return m
}
func unfoldICS(s string) []string {
	var out []string
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		line := sc.Text()
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') && len(out) > 0 {
			out[len(out)-1] += strings.TrimLeft(line, " \t")
		} else {
			out = append(out, line)
		}
	}
	return out
}
func firstProp(m map[string][]string, k string) string {
	if v := m[k]; len(v) > 0 {
		return v[0]
	}
	return ""
}
func parseICSTime(s string) (*time.Time, bool) {
	if s == "" {
		return nil, false
	}
	layouts := []string{"20060102T150405Z", "20060102T150405", "20060102"}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			tt := t
			return &tt, l == "20060102"
		}
	}
	return nil, false
}
func icsUnescape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(s, `\n`, "\n"), `\,`, ","), `\;`, ";"), `\\`, `\`)
}
func htmlUnescape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(s, "&lt;", "<"), "&gt;", ">"), "&amp;", "&"), "&#13;", "")
}

func (a *App) executeCalendarAction(ctx context.Context, r connectedResource, action string, input map[string]any) (map[string]any, error) {
	switch action {
	case "calendar.create", "calendar.update":
		return a.caldavPut(r, input)
	case "calendar.delete":
		return a.caldavDelete(r, input)
	default:
		return nil, fmt.Errorf("unsupported calendar action %s", action)
	}
}

func (a *App) caldavPut(r connectedResource, input map[string]any) (map[string]any, error) {
	base := strings.TrimRight(secretString(r, "caldav_url"), "/")
	if base == "" {
		base = strings.TrimRight(r.Locator, "/")
	}
	uid := stringAny(input["uid"])
	if uid == "" {
		uid = randHex(12) + "@shu"
	}
	ical := buildICS(uid, stringAny(input["summary"]), stringAny(input["description"]), stringAny(input["location"]), stringAny(input["dtstart"]), stringAny(input["dtend"]))
	req, err := http.NewRequest("PUT", base+"/"+uid+".ics", strings.NewReader(ical))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/calendar")
	if u, p := secretString(r, "caldav_user"), secretString(r, "caldav_password"); u != "" || p != "" {
		req.SetBasicAuth(u, p)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("caldav PUT HTTP %d", resp.StatusCode)
	}
	return map[string]any{"uid": uid, "put": true}, nil
}
func (a *App) caldavDelete(r connectedResource, input map[string]any) (map[string]any, error) {
	base := strings.TrimRight(secretString(r, "caldav_url"), "/")
	if base == "" {
		base = strings.TrimRight(r.Locator, "/")
	}
	uid := stringAny(input["uid"])
	if uid == "" {
		return nil, fmt.Errorf("uid required")
	}
	req, err := http.NewRequest("DELETE", base+"/"+uid+".ics", nil)
	if err != nil {
		return nil, err
	}
	if u, p := secretString(r, "caldav_user"), secretString(r, "caldav_password"); u != "" || p != "" {
		req.SetBasicAuth(u, p)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("caldav DELETE HTTP %d", resp.StatusCode)
	}
	return map[string]any{"uid": uid, "deleted": true}, nil
}
func buildICS(uid, summary, desc, loc, start, end string) string {
	if start == "" {
		start = time.Now().UTC().Format("20060102T150405Z")
	} else {
		start = normalizeICSTime(start)
	}
	if end == "" {
		end = time.Now().Add(time.Hour).UTC().Format("20060102T150405Z")
	} else {
		end = normalizeICSTime(end)
	}
	return fmt.Sprintf("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Shu//Connected//EN\r\nBEGIN:VEVENT\r\nUID:%s\r\nSUMMARY:%s\r\nDESCRIPTION:%s\r\nLOCATION:%s\r\nDTSTART:%s\r\nDTEND:%s\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n", uid, icsEscape(summary), icsEscape(desc), icsEscape(loc), start, end)
}
func normalizeICSTime(s string) string {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC().Format("20060102T150405Z")
	}
	return s
}
func icsEscape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(s, "\\", "\\\\"), ";", "\\;"), ",", "\\,")
}

func secretString(r connectedResource, k string) string  { return stringAny(r.Secrets[k]) }
func secretInt(r connectedResource, k string, d int) int { return intAny(r.Secrets[k], d) }
func secretBool(r connectedResource, k string, d bool) bool {
	if v, ok := r.Secrets[k]; ok {
		return boolAny(v)
	}
	return d
}
func metaValue(m map[string]any, k string) any {
	if m == nil {
		return nil
	}
	return m[k]
}
func metaString(m map[string]any, k string) string  { return stringAny(metaValue(m, k)) }
func metaInt(m map[string]any, k string, d int) int { return intAny(metaValue(m, k), d) }
func stringAny(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	case float64:
		return strconv.FormatInt(int64(x), 10)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case uint32:
		return strconv.FormatUint(uint64(x), 10)
	}
	return ""
}
func intAny(v any, d int) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case int64:
		return int(x)
	case string:
		if n, err := strconv.Atoi(x); err == nil {
			return n
		}
	}
	return d
}
func boolAny(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return strings.EqualFold(x, "true") || x == "1"
	case float64:
		return x != 0
	case int:
		return x != 0
	}
	return false
}
func stringSlice(v any) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		out := []string{}
		for _, v := range x {
			if s := stringAny(v); s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		if x == "" {
			return nil
		}
		return []string{x}
	}
	return nil
}
func ptrTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

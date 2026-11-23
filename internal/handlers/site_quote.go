package handlers

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// ---------- Scoper spec (schema v1) ----------

type specItem struct {
	T     string `json:"t"`
	Later bool   `json:"later"`
}

type quoteSpec struct {
	V          int        `json:"v"`
	Name       string     `json:"name"`
	Problem    string     `json:"problem"`
	Core       string     `json:"core"`
	Roles      []string   `json:"roles"`
	Screens    []specItem `json:"screens"`
	Data       []specItem `json:"data"`
	Integ      []specItem `json:"integ"`
	Login      string     `json:"login"`
	Pay        []string   `json:"pay"`
	PayRule    string     `json:"payrule"`
	Notify     []string   `json:"notify"`
	NotifyRule string     `json:"notifyrule"`
	UI         string     `json:"ui"`
	Device     string     `json:"device"`
	Like       string     `json:"like"`
	Brand      string     `json:"brand"`
	Stack      string     `json:"stack"`
	StackOther string     `json:"stackother"`
	Have       []string   `json:"have"`
	HaveLink   string     `json:"havelink"`
	Done       []string   `json:"done"`
	NotNow     string     `json:"notnow"`
	When       string     `json:"when"`
	Email      string     `json:"email,omitempty"` // optional; only prefills the /start form, never used for pricing
}

const (
	specMaxList = 20
	specMaxItem = 160
)

func enum(vals ...string) map[string]bool {
	m := map[string]bool{}
	for _, v := range vals {
		m[v] = true
	}
	return m
}

var (
	specLogin   = enum("none", "email", "google", "phone")
	specPay     = enum("codi", "card", "cash", "mon")
	specNotify  = enum("email", "whatsapp", "sms")
	specUI      = enum("es", "en", "both")
	specDevice  = enum("phone", "desk", "pwa", "native")
	specStack   = enum("auto", "next", "go", "py", "php", "other")
	specHave    = enum("nothing", "site", "repo", "sheet")
	quoteRegion = map[string]string{"MX": "MX", "US": "INTL", "CA": "INTL", "EU": "INTL", "GULF": "INTL", "OTHER": "ROW"}
)

// clipRunes trims and cuts to n characters without splitting a UTF-8 sequence.
func clipRunes(s string, n int) string {
	s = strings.TrimSpace(strings.ToValidUTF8(s, ""))
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return strings.TrimSpace(string([]rune(s)[:n]))
}

func clipList(in []string) []string {
	out := []string{}
	for _, s := range in {
		if s = clipRunes(s, specMaxItem); s != "" && len(out) < specMaxList {
			out = append(out, s)
		}
	}
	return out
}

func clipItems(in []specItem) []specItem {
	out := []specItem{}
	for _, it := range in {
		if t := clipRunes(it.T, specMaxItem); t != "" && len(out) < specMaxList {
			out = append(out, specItem{T: t, Later: it.Later})
		}
	}
	return out
}

func enumList(field string, in []string, allowed map[string]bool) ([]string, error) {
	out, seen := []string{}, map[string]bool{}
	for _, v := range in {
		if !allowed[v] {
			return nil, fmt.Errorf("spec.%s: unknown value %q", field, clipRunes(v, 40))
		}
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out, nil
}

// parseSpec decodes and validates a v1 spec strictly: unknown fields and unknown enum
// values are rejected, free text is clipped and lists are capped.
func parseSpec(raw []byte) (quoteSpec, error) {
	var s quoteSpec
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		return s, fmt.Errorf("spec: %v", err)
	}
	if dec.More() {
		return s, errors.New("spec: trailing data")
	}
	if s.V != 1 {
		return s, errors.New("spec: unsupported version")
	}
	one := []struct {
		field, val string
		allowed    map[string]bool
	}{{"login", s.Login, specLogin}, {"ui", s.UI, specUI}, {"device", s.Device, specDevice}, {"stack", s.Stack, specStack}}
	for _, f := range one {
		if !f.allowed[f.val] {
			return s, fmt.Errorf("spec.%s: unknown value %q", f.field, clipRunes(f.val, 40))
		}
	}
	var err error
	if s.Pay, err = enumList("pay", s.Pay, specPay); err != nil {
		return s, err
	}
	if s.Notify, err = enumList("notify", s.Notify, specNotify); err != nil {
		return s, err
	}
	if s.Have, err = enumList("have", s.Have, specHave); err != nil {
		return s, err
	}
	s.Name, s.Problem, s.Core = clipRunes(s.Name, 120), clipRunes(s.Problem, 2000), clipRunes(s.Core, 2000)
	s.PayRule, s.NotifyRule = clipRunes(s.PayRule, 600), clipRunes(s.NotifyRule, 600)
	s.Like, s.Brand, s.StackOther = clipRunes(s.Like, 600), clipRunes(s.Brand, 600), clipRunes(s.StackOther, 120)
	s.HaveLink, s.NotNow, s.When = clipRunes(s.HaveLink, 300), clipRunes(s.NotNow, 1200), clipRunes(s.When, 160)
	s.Email = clipRunes(s.Email, 200)
	s.Roles, s.Done = clipList(s.Roles), clipList(s.Done)
	s.Screens, s.Data, s.Integ = clipItems(s.Screens), clipItems(s.Data), clipItems(s.Integ)
	if s.Name == "" {
		return s, errors.New("spec.name: required")
	}
	return s, nil
}

// ---------- Size points and price (deterministic; the AI never sets these) ----------

// specPoints sizes a spec. manual is true when the spec cannot be priced automatically.
func specPoints(s quoteSpec) (points int, manual bool) {
	if len(s.Roles) > 1 {
		points += len(s.Roles) - 1
	}
	for _, it := range s.Screens {
		if !it.Later {
			points++
		}
	}
	for _, it := range s.Data {
		if !it.Later {
			points++
		}
	}
	for _, it := range s.Integ {
		if !it.Later {
			points += 2
		}
	}
	switch s.Login {
	case "email", "google":
		points++
	case "phone":
		points += 2
	}
	for _, p := range s.Pay {
		points += map[string]int{"codi": 2, "card": 2, "cash": 0, "mon": 3}[p]
	}
	for _, n := range s.Notify {
		points += map[string]int{"email": 1, "whatsapp": 2, "sms": 2}[n]
	}
	if s.UI == "both" {
		points++
	}
	switch s.Device {
	case "pwa":
		points++
	case "native":
		manual = true
	}
	return points, manual
}

// highHostingTier: anything that takes payments (other than cash) or has accounts costs more to run.
func highHostingTier(s quoteSpec) bool {
	if s.Login != "none" {
		return true
	}
	for _, p := range s.Pay {
		if p != "cash" {
			return true
		}
	}
	return false
}

type regionPrice struct {
	Credit      int `json:"credit"`
	HostingLow  int `json:"hosting_low"`
	HostingHigh int `json:"hosting_high"`
}

type pricingConfig struct {
	BaseCredits int `json:"base_credits"`
	PerPoint    int `json:"per_point"`
	MinCredits  int `json:"min_credits"`
	Regions     struct {
		MX   regionPrice `json:"MX"`
		INTL regionPrice `json:"INTL"`
		ROW  regionPrice `json:"ROW"`
	} `json:"regions"`
}

func defaultPricing() pricingConfig {
	var p pricingConfig
	p.BaseCredits, p.PerPoint, p.MinCredits = 12, 3, 15
	p.Regions.MX = regionPrice{Credit: 450, HostingLow: 299, HostingHigh: 499} // MXN, IVA included
	p.Regions.INTL = regionPrice{Credit: 60, HostingLow: 29, HostingHigh: 49}  // USD
	p.Regions.ROW = regionPrice{Credit: 35, HostingLow: 19, HostingHigh: 29}   // USD
	return p
}

func (p pricingConfig) validate() error {
	if p.BaseCredits < 0 || p.BaseCredits > 500 || p.PerPoint < 0 || p.PerPoint > 100 || p.MinCredits < 0 || p.MinCredits > 1000 {
		return errors.New("base_credits 0-500, per_point 0-100, min_credits 0-1000")
	}
	for name, r := range map[string]regionPrice{"MX": p.Regions.MX, "INTL": p.Regions.INTL, "ROW": p.Regions.ROW} {
		if r.Credit < 1 || r.Credit > 1_000_000 || r.HostingLow < 0 || r.HostingHigh < r.HostingLow || r.HostingHigh > 100_000 {
			return fmt.Errorf("regions.%s: credit 1-1000000, 0 <= hosting_low <= hosting_high <= 100000", name)
		}
	}
	return nil
}

func (p pricingConfig) region(name string) regionPrice {
	switch name {
	case "MX":
		return p.Regions.MX
	case "INTL":
		return p.Regions.INTL
	}
	return p.Regions.ROW
}

type quotePrice struct {
	Region   string
	Currency string
	Points   int
	Credits  int
	Price    int
	Monthly  int
	Manual   bool
}

// priceQuote is the only place a price is computed.
func priceQuote(p pricingConfig, s quoteSpec, country string) quotePrice {
	points, manual := specPoints(s)
	region := quoteRegion[country]
	if region == "" {
		region = "ROW"
	}
	q := quotePrice{Region: region, Currency: "USD", Points: points, Manual: manual}
	if region == "MX" {
		q.Currency = "MXN"
	}
	if manual {
		return q // native apps are priced by hand
	}
	rp := p.region(region)
	q.Credits = max(p.MinCredits, p.BaseCredits+p.PerPoint*points)
	q.Price = q.Credits * rp.Credit
	q.Monthly = rp.HostingLow
	if highHostingTier(s) {
		q.Monthly = rp.HostingHigh
	}
	return q
}

func thousands(n int) string {
	s := strconv.Itoa(n)
	if n < 0 {
		return "-" + thousands(-n)
	}
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String()
}

func money(currency string, n int) string { return currency + " " + thousands(n) }

// priceBlock is the code-built price text. The AI is told to copy it verbatim; if it does
// not, enforcePrice puts it back.
func priceBlock(q quotePrice, lang string) string {
	if q.Manual {
		if lang == "es" {
			return "Precio: lo confirmamos por separado después de revisar el alcance de la app nativa."
		}
		return "Price: we confirm it separately after reviewing the native app scope."
	}
	tax := ""
	if q.Currency == "MXN" {
		tax = ", IVA incluido"
		if lang != "es" {
			tax = ", IVA incluido (tax included)"
		}
	}
	if lang == "es" {
		return fmt.Sprintf("Precio de la primera versión: %d créditos = %s%s.\nHosting mensual: %s%s (hosting, dominio, respaldos y actualizaciones de seguridad).",
			q.Credits, money(q.Currency, q.Price), tax, money(q.Currency, q.Monthly), tax)
	}
	return fmt.Sprintf("Price for the first version: %d credits = %s%s.\nMonthly hosting: %s%s (hosting, domain, backups and security updates).",
		q.Credits, money(q.Currency, q.Price), tax, money(q.Currency, q.Monthly), tax)
}

var reMoney = regexp.MustCompile(`(?i)\b(?:MXN|USD|EUR|US\$)\s?\$?\d[\d,.]*|\$\s?\d[\d,.]*|\d[\d,.]*\s?(?:MXN|USD|EUR|pesos|d[oó]lares|dollars)\b`)

// enforcePrice checks that an email states exactly the code price: both amounts present,
// the credit count present, and no other money amount anywhere. On mismatch it removes
// every line that mentions money and puts the code-built price block in their place.
func enforcePrice(email string, q quotePrice, lang string) (string, bool) {
	allowed := map[string]bool{}
	if !q.Manual {
		allowed[money(q.Currency, q.Price)] = true
		allowed[money(q.Currency, q.Monthly)] = true
	}
	ok := true
	for _, m := range reMoney.FindAllString(email, -1) {
		if !allowed[strings.TrimRight(m, ",.")] {
			ok = false
		}
	}
	for a := range allowed {
		if !strings.Contains(email, a) {
			ok = false
		}
	}
	if !q.Manual {
		word := " credits"
		if lang == "es" {
			word = " créditos"
		}
		if !strings.Contains(email, strconv.Itoa(q.Credits)+word) {
			ok = false
		}
	}
	if ok {
		return email, false
	}
	credit := regexp.MustCompile(`(?i)\d+\s+(?:credits?|cr[eé]ditos?)`)
	var out []string
	placed := false
	for _, line := range strings.Split(email, "\n") {
		if reMoney.MatchString(line) || credit.MatchString(line) {
			if !placed {
				out = append(out, priceBlock(q, lang))
				placed = true
			}
			continue
		}
		out = append(out, line)
	}
	if !placed {
		out = append(out, "", priceBlock(q, lang))
	}
	return strings.Join(out, "\n"), true
}

func specLang(s quoteSpec) string {
	if s.UI == "es" {
		return "es"
	}
	return "en"
}

// renderSpecText is the plain-text copy stored as the inquiry message and shown to the owner.
func renderSpecText(s quoteSpec) string {
	var b strings.Builder
	line := func(k, v string) {
		if v != "" {
			fmt.Fprintf(&b, "%s: %s\n", k, v)
		}
	}
	list := func(k string, v []string) {
		if len(v) > 0 {
			fmt.Fprintf(&b, "%s:\n", k)
			for _, x := range v {
				fmt.Fprintf(&b, "  - %s\n", x)
			}
		}
	}
	items := func(k string, v []specItem) {
		if len(v) > 0 {
			fmt.Fprintf(&b, "%s:\n", k)
			for _, x := range v {
				later := ""
				if x.Later {
					later = " (later)"
				}
				fmt.Fprintf(&b, "  - %s%s\n", x.T, later)
			}
		}
	}
	line("Project", s.Name)
	line("Problem", s.Problem)
	line("Core action", s.Core)
	list("Roles", s.Roles)
	items("Screens", s.Screens)
	items("Data", s.Data)
	items("Integrations", s.Integ)
	line("Login", s.Login)
	line("Payments", strings.Join(s.Pay, ", "))
	line("Payment rule", s.PayRule)
	line("Notifications", strings.Join(s.Notify, ", "))
	line("Notification rule", s.NotifyRule)
	line("Language", s.UI)
	line("Device", s.Device)
	line("Looks like", s.Like)
	line("Brand", s.Brand)
	stack := s.Stack
	if s.Stack == "other" && s.StackOther != "" {
		stack += " (" + s.StackOther + ")"
	}
	line("Stack", stack)
	line("Already has", strings.Join(s.Have, ", "))
	line("Link", s.HaveLink)
	list("Done when", s.Done)
	line("Not now", s.NotNow)
	line("When", s.When)
	return b.String()
}

// ---------- POST /api/v1/site/quote-request ----------

type quoteRequestIn struct {
	Spec    json.RawMessage `json:"spec"`
	Name    string          `json:"name"`
	Email   string          `json:"email"`
	Country string          `json:"country"`
	Website string          `json:"website"` // honeypot
}

type quoteClient struct{ Name, Email, Country string }

func (h *SiteHandler) loadPricing() (pricingConfig, error) {
	var raw string
	err := h.DB.Get(&raw, `SELECT value FROM site_settings WHERE key = 'pricing'`)
	if errors.Is(err, sql.ErrNoRows) {
		return defaultPricing(), nil
	}
	if err != nil {
		return pricingConfig{}, err
	}
	var p pricingConfig
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return pricingConfig{}, err
	}
	return p, p.validate()
}

// QuoteRequest handles POST /api/v1/site/quote-request.
func (h *SiteHandler) QuoteRequest(w http.ResponseWriter, r *http.Request) {
	var in quoteRequestIn
	dec := json.NewDecoder(io.LimitReader(r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "Send the request as JSON under 64 KB.")
		return
	}
	if in.Website != "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true}) // bot: accept silently, store nothing
		return
	}
	in.Name, in.Email = clipRunes(in.Name, 120), clip(in.Email, 200)
	if in.Name == "" {
		writeError(w, http.StatusBadRequest, "Add your name.")
		return
	}
	if a, err := mail.ParseAddress(in.Email); err != nil || a.Address != in.Email {
		writeError(w, http.StatusBadRequest, "Check your email address.")
		return
	}
	if _, ok := quoteRegion[in.Country]; !ok {
		writeError(w, http.StatusBadRequest, "Choose where this will be invoiced.")
		return
	}
	spec, err := parseSpec(in.Spec)
	if err != nil {
		writeError(w, http.StatusBadRequest, "The project spec is invalid ("+err.Error()+"). Go back to the scoper and send it again.")
		return
	}
	cfg, err := h.loadPricing()
	if err != nil {
		log.Printf("quote pricing unavailable: %v", err)
		writeError(w, http.StatusServiceUnavailable, "Quotes are unavailable right now. Try again later.")
		return
	}
	if h.overDailyCap("site_inquiries", maxInquiriesPerDay) {
		writeError(w, http.StatusTooManyRequests, "We're receiving a lot of requests right now. Try again tomorrow.")
		return
	}
	q := priceQuote(cfg, spec, in.Country)
	lang := specLang(spec)
	text := clipRunes(renderSpecText(spec), 32000)

	tx, err := h.DB.Beginx()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Your request didn't save. Try again.")
		return
	}
	defer tx.Rollback()
	res, err := tx.Exec(`INSERT INTO site_inquiries (name, email, project_type, timeline, repo, message, source)
		VALUES (?, ?, 'Scoped project', ?, ?, ?, 'scoper')`, in.Name, in.Email, spec.When, spec.HaveLink, text)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Your request didn't save. Try again.")
		return
	}
	inqID, _ := res.LastInsertId()
	res, err = tx.Exec(`INSERT INTO site_quotes (inquiry_id, country, lang, region, currency, points, credits, price, monthly, manual)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, inqID, in.Country, lang, q.Region, q.Currency, q.Points, q.Credits, q.Price, q.Monthly, q.Manual)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Your request didn't save. Try again.")
		return
	}
	quoteID, _ := res.LastInsertId()
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "Your request didn't save. Try again.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	client := quoteClient{Name: in.Name, Email: in.Email, Country: in.Country}
	h.bg.Add(1)
	go func() {
		defer h.bg.Done()
		h.finishQuote(quoteID, inqID, spec, text, q, lang, client)
	}()
}

// ---------- AI draft, approval token, owner notification ----------

const (
	maxQuoteDraftsPerDay = 40
	quoteDraftTimeout    = 90 * time.Second
)

// reserveAIDraft claims one slot of the daily AI budget for this quote in a single
// statement, so concurrent requests cannot overshoot. Any error means no draft.
func (h *SiteHandler) reserveAIDraft(id int64) bool {
	if h.Drafter == nil {
		return false
	}
	res, err := h.DB.Exec(`UPDATE site_quotes SET ai_at = CURRENT_TIMESTAMP WHERE id = ? AND ai_at IS NULL
		AND (SELECT COUNT(*) FROM site_quotes WHERE ai_at > datetime('now','-1 day')) < ?`, id, maxQuoteDraftsPerDay)
	if err != nil {
		log.Printf("quote #%d: AI cap check failed: %v", id, err)
		return false
	}
	n, err := res.RowsAffected()
	return err == nil && n == 1
}

func clipDraft(d QuoteDraft) QuoteDraft {
	d.Summary, d.ClientEmail = clipRunes(d.Summary, 3000), clipRunes(d.ClientEmail, 12000)
	cl := func(v []string) []string {
		out := []string{}
		for _, s := range v {
			if s = clipRunes(s, 400); s != "" && len(out) < 30 {
				out = append(out, s)
			}
		}
		return out
	}
	d.Included, d.Questions, d.Flags = cl(d.Included), cl(d.Questions), cl(d.Flags)
	return d
}

func newApproveToken() (token, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, tokenHash(token), nil
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (h *SiteHandler) finishQuote(id, inquiryID int64, spec quoteSpec, specText string, q quotePrice, lang string, c quoteClient) {
	draft := QuoteDraft{Included: []string{}, Questions: []string{}, Flags: []string{}}
	flags := []string{}
	var inTok, outTok int64
	if h.reserveAIDraft(id) {
		ctx, cancel := context.WithTimeout(context.Background(), quoteDraftTimeout)
		system, user := buildQuotePrompt(spec, q, lang)
		d, i, o, err := h.Drafter.Draft(ctx, system, user)
		cancel()
		inTok, outTok = i, o
		if err != nil {
			log.Printf("quote #%d: AI draft failed: %v", id, err)
			flags = append(flags, "no AI draft")
		} else {
			draft = clipDraft(d)
			var mismatch bool
			if draft.ClientEmail, mismatch = enforcePrice(draft.ClientEmail, q, lang); mismatch {
				flags = append(flags, "price mismatch")
			}
			flags = append(flags, draft.Flags...)
		}
	} else {
		flags = append(flags, "no AI draft")
	}
	if q.Manual {
		flags = append([]string{"manual price (native app)"}, flags...)
	}

	token, hash, err := newApproveToken()
	if err != nil {
		log.Printf("quote #%d: token: %v", id, err)
		return
	}
	draftJSON, _ := json.Marshal(draft)
	flagsJSON, _ := json.Marshal(flags)
	if _, err := h.DB.Exec(`UPDATE site_quotes SET draft = ?, flags = ?, input_tokens = ?, output_tokens = ?, approve_token_hash = ? WHERE id = ?`,
		string(draftJSON), string(flagsJSON), inTok, outTok, hash, id); err != nil {
		log.Printf("quote #%d: save draft: %v", id, err)
		return
	}
	m := h.mail()
	if !m.ready() {
		log.Printf("quote #%d stored; SMTP not configured, no review link sent", id)
		return
	}
	link := siteAPIBase + "/api/v1/site/admin/quotes/" + strconv.FormatInt(id, 10) + "?t=" + token
	var b strings.Builder
	fmt.Fprintf(&b, "New scoped quote #%d (inquiry #%d)\n\n", id, inquiryID)
	fmt.Fprintf(&b, "Client: %s <%s>\nCountry: %s\nRegion: %s (%s)\nPoints: %d\nCredits: %d\nPrice: %s\nMonthly: %s\nManual: %v\n",
		c.Name, c.Email, c.Country, q.Region, q.Currency, q.Points, q.Credits, money(q.Currency, q.Price), money(q.Currency, q.Monthly), q.Manual)
	fmt.Fprintf(&b, "Flags: %s\nTokens: %d in / %d out\n\n", strings.Join(flags, "; "), inTok, outTok)
	fmt.Fprintf(&b, "Review, edit and send (link works once):\n%s\n\n", link)
	if draft.ClientEmail != "" {
		fmt.Fprintf(&b, "--- AI draft ---\nSummary: %s\n", draft.Summary)
		for _, s := range draft.Included {
			fmt.Fprintf(&b, "Included: %s\n", s)
		}
		for _, s := range draft.Questions {
			fmt.Fprintf(&b, "Question: %s\n", s)
		}
		fmt.Fprintf(&b, "\nClient email:\n%s\n\n", draft.ClientEmail)
	}
	fmt.Fprintf(&b, "--- Spec ---\n%s", specText)
	if err := m.deliver("EmpowerTours site", h.SMTP.To, c.Email, fmt.Sprintf("Quote draft #%d: %s", id, oneLine(spec.Name, 80)), b.String()); err != nil {
		log.Printf("quote #%d: owner email: %v", id, err)
	}
}

func oneLine(s string, n int) string {
	return clipRunes(strings.Join(strings.Fields(s), " "), n)
}

// ---------- Mail seam (tests replace it) ----------

type siteMailer interface {
	ready() bool
	deliver(fromName, to, replyTo, subject, body string) error
}

func (h *SiteHandler) mail() siteMailer {
	if h.mailer != nil {
		return h.mailer
	}
	return h.SMTP
}

// ---------- Pricing admin (bearer SITE_ADMIN_TOKEN) ----------

// GetPricing handles GET /api/v1/site/admin/pricing.
func (h *SiteHandler) GetPricing(w http.ResponseWriter, r *http.Request) {
	if err := h.authorized(r); err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	p, err := h.loadPricing()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "pricing unreadable")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// PutPricing handles PUT /api/v1/site/admin/pricing. The whole object is replaced.
func (h *SiteHandler) PutPricing(w http.ResponseWriter, r *http.Request) {
	if err := h.authorized(r); err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var p pricingConfig
	dec := json.NewDecoder(io.LimitReader(r.Body, 8<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "send the full pricing object as JSON")
		return
	}
	if err := p.validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	raw, _ := json.Marshal(p)
	if _, err := h.DB.Exec(`INSERT INTO site_settings (key, value, updated_at) VALUES ('pricing', ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`, string(raw)); err != nil {
		writeError(w, http.StatusInternalServerError, "pricing not saved")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// RedactQuoteToken keeps quote tokens (owner review, client accept, owner pay-verify) out
// of request logs. It must run before the request logger; handlers read r.URL, which is
// left untouched.
func RedactQuoteToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if (strings.HasPrefix(r.URL.Path, "/api/v1/site/admin/quotes/") || strings.HasPrefix(r.URL.Path, "/api/v1/site/quotes/")) && r.URL.RawQuery != "" {
			r.RequestURI = r.URL.Path + "?t=REDACTED"
		}
		next.ServeHTTP(w, r)
	})
}

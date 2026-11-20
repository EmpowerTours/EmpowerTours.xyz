package handlers

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
)

type quoteRow struct {
	ID        int64  `db:"id"`
	Country   string `db:"country"`
	Lang      string `db:"lang"`
	Region    string `db:"region"`
	Currency  string `db:"currency"`
	Points    int    `db:"points"`
	Credits   int    `db:"credits"`
	Price     int    `db:"price"`
	Monthly   int    `db:"monthly"`
	Manual    bool   `db:"manual"`
	Status    string `db:"status"`
	Draft     string `db:"draft"`
	Flags     string `db:"flags"`
	TokenHash string `db:"approve_token_hash"`
	Name      string `db:"name"`
	Email     string `db:"email"`
	SpecText  string `db:"message"`
}

// Compared against when the id does not exist, so a miss costs the same as a wrong token.
var dummyTokenHash = strings.Repeat("0", 64)

// quoteByToken returns the quote only if the token matches its stored hash.
func (h *SiteHandler) quoteByToken(idParam, token string) (*quoteRow, bool) {
	var row quoteRow
	id, err := strconv.ParseInt(idParam, 10, 64)
	found := err == nil && id > 0 && h.DB.Get(&row, `SELECT q.id, q.country, q.lang, q.region, q.currency, q.points, q.credits, q.price, q.monthly,
		q.manual, q.status, q.draft, q.flags, q.approve_token_hash, i.name, i.email, i.message
		FROM site_quotes q JOIN site_inquiries i ON i.id = q.inquiry_id WHERE q.id = ?`, id) == nil
	stored := row.TokenHash
	if !found || len(stored) != 64 {
		stored = dummyTokenHash
	}
	match := subtle.ConstantTimeCompare([]byte(tokenHash(token)), []byte(stored)) == 1
	if !found || !match || token == "" {
		return nil, false
	}
	return &row, true
}

func setQuotePageHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

var quotePageTmpl = template.Must(template.New("q").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex"><title>Quote #{{.ID}}</title>
<style>body{font:15px/1.5 system-ui,sans-serif;max-width:860px;margin:24px auto;padding:0 16px;color:#111;background:#fff}
pre{white-space:pre-wrap;background:#f4f4f6;padding:12px;border-radius:8px}label{display:block;margin-top:12px;font-weight:600}
input,textarea{width:100%;box-sizing:border-box;font:inherit;padding:8px}textarea{min-height:320px}.flags{color:#a00}
button{margin-top:16px;padding:10px 16px;font:inherit;margin-right:8px}</style></head><body>
<h1>Quote #{{.ID}}{{if .Status}} — {{.Status}}{{end}}</h1>
{{if .Message}}<p>{{.Message}}</p>{{end}}
{{if .Form}}
<p><b>{{.Name}}</b> &lt;{{.Email}}&gt; · country {{.Country}} · region {{.Region}} ({{.Currency}}) · {{.Points}} points{{if .Manual}} · <b>manual price</b>{{end}}</p>
{{if .Flags}}<p class="flags">Flags: {{range $i, $f := .Flags}}{{if $i}}; {{end}}{{$f}}{{end}}</p>{{end}}
{{if .Draft.Summary}}<h2>AI summary</h2><p>{{.Draft.Summary}}</p>{{end}}
{{if .Draft.Included}}<h2>Included</h2><ul>{{range .Draft.Included}}<li>{{.}}</li>{{end}}</ul>{{end}}
{{if .Draft.Questions}}<h2>Open questions</h2><ul>{{range .Draft.Questions}}<li>{{.}}</li>{{end}}</ul>{{end}}
<h2>Spec</h2><pre>{{.SpecText}}</pre>
<h2>Send to client</h2>
<form method="post">
<input type="hidden" name="t" value="{{.Token}}">
<label>Credits <input name="credits" inputmode="numeric" value="{{.Credits}}"></label>
<label>Price ({{.Currency}}{{if eq .Currency "MXN"}}, IVA included{{end}}) <input name="price" inputmode="numeric" value="{{.Price}}"></label>
<label>Monthly ({{.Currency}}) <input name="monthly" inputmode="numeric" value="{{.Monthly}}"></label>
<label>Subject <input name="subject" maxlength="200" value="{{.Subject}}"></label>
<label>Email body <textarea name="body">{{.Body}}</textarea></label>
<p>The credits/price/monthly fields are recorded on the quote; the client sees only the email body, so keep them in sync.</p>
<button name="action" value="send">Send to {{.Email}}</button><button name="action" value="discard">Discard</button>
</form>
{{end}}
</body></html>`))

type quotePageData struct {
	quoteRow
	Form           bool
	Message, Token string
	Subject, Body  string
	Flags          []string
	Draft          QuoteDraft
}

func renderQuotePage(w http.ResponseWriter, code int, d quotePageData) {
	setQuotePageHeaders(w)
	w.WriteHeader(code)
	if err := quotePageTmpl.Execute(w, d); err != nil {
		log.Printf("quote page: %v", err)
	}
}

func quoteNotFound(w http.ResponseWriter) {
	setQuotePageHeaders(w)
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte("<!doctype html><title>Not found</title><p>Not found.</p>"))
}

func defaultSubject(row *quoteRow) string {
	name := ""
	if i := strings.Index(row.SpecText, "Project: "); i >= 0 {
		name = strings.SplitN(row.SpecText[i+len("Project: "):], "\n", 2)[0]
	}
	if row.Lang == "es" {
		return oneLine("Tu cotización de EmpowerTours: "+name, 200)
	}
	return oneLine("Your EmpowerTours quote: "+name, 200)
}

// QuotePage handles GET /api/v1/site/admin/quotes/{id}?t=… It only reads: a GET never
// sends, spends or changes anything.
func (h *SiteHandler) QuotePage(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("t")
	row, ok := h.quoteByToken(chi.URLParam(r, "id"), token)
	if !ok {
		quoteNotFound(w)
		return
	}
	if row.Status != "draft" {
		renderQuotePage(w, http.StatusGone, quotePageData{quoteRow: *row, Message: "This link has been used. The quote is " + row.Status + "."})
		return
	}
	var d QuoteDraft
	json.Unmarshal([]byte(row.Draft), &d)
	var flags []string
	json.Unmarshal([]byte(row.Flags), &flags)
	body := d.ClientEmail
	if body == "" {
		body = priceBlock(quotePrice{Currency: row.Currency, Credits: row.Credits, Price: row.Price, Monthly: row.Monthly, Manual: row.Manual}, row.Lang)
	}
	renderQuotePage(w, http.StatusOK, quotePageData{quoteRow: *row, Form: true, Token: token, Subject: defaultSubject(row), Body: body, Flags: flags, Draft: d})
}

func formInt(r *http.Request, name string, lo, hi int) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(strings.ReplaceAll(r.PostForm.Get(name), ",", "")))
	if err != nil || n < lo || n > hi {
		return 0, fmt.Errorf("%s must be a whole number from %d to %d", name, lo, hi)
	}
	return n, nil
}

// QuoteAction handles POST /api/v1/site/admin/quotes/{id} (form: t, action=send|discard, …).
// The token works once: any action moves the quote out of "draft", after which every
// further POST gets 410.
func (h *SiteHandler) QuoteAction(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	row, ok := h.quoteByToken(chi.URLParam(r, "id"), r.PostForm.Get("t"))
	if !ok {
		quoteNotFound(w)
		return
	}
	gone := func() {
		renderQuotePage(w, http.StatusGone, quotePageData{quoteRow: quoteRow{ID: row.ID}, Message: "This link has already been used."})
	}
	if row.Status != "draft" {
		gone()
		return
	}
	switch r.PostForm.Get("action") {
	case "discard":
		res, err := h.DB.Exec(`UPDATE site_quotes SET status = 'discarded' WHERE id = ? AND status = 'draft'`, row.ID)
		if err != nil {
			http.Error(w, "not saved", http.StatusInternalServerError)
			return
		}
		if n, _ := res.RowsAffected(); n != 1 {
			gone()
			return
		}
		renderQuotePage(w, http.StatusOK, quotePageData{quoteRow: quoteRow{ID: row.ID, Status: "discarded"}, Message: "Discarded. Nothing was sent to the client."})
	case "send":
		h.sendQuote(w, r, row)
	default:
		http.Error(w, "unknown action", http.StatusBadRequest)
	}
}

func (h *SiteHandler) sendQuote(w http.ResponseWriter, r *http.Request, row *quoteRow) {
	bad := func(msg string) {
		renderQuotePage(w, http.StatusBadRequest, quotePageData{quoteRow: quoteRow{ID: row.ID}, Message: msg + " Go back and fix it; the link still works."})
	}
	credits, err := formInt(r, "credits", 0, 100_000)
	if err != nil {
		bad(err.Error())
		return
	}
	price, err := formInt(r, "price", 0, 100_000_000)
	if err != nil {
		bad(err.Error())
		return
	}
	monthly, err := formInt(r, "monthly", 0, 10_000_000)
	if err != nil {
		bad(err.Error())
		return
	}
	subject := strings.TrimSpace(r.PostForm.Get("subject"))
	body := strings.TrimSpace(strings.ReplaceAll(r.PostForm.Get("body"), "\r\n", "\n"))
	if subject == "" || utf8.RuneCountInString(subject) > 200 || strings.ContainsAny(subject, "\r\n") {
		bad("Subject must be one line, 1 to 200 characters.")
		return
	}
	if body == "" || utf8.RuneCountInString(body) > 20000 {
		bad("Email body must be 1 to 20,000 characters.")
		return
	}
	m := h.mail()
	if !m.ready() {
		renderQuotePage(w, http.StatusServiceUnavailable, quotePageData{quoteRow: quoteRow{ID: row.ID}, Message: "Email is not configured on the server. Nothing was sent; the link still works."})
		return
	}
	// Claim the quote first so two concurrent POSTs cannot both send.
	res, err := h.DB.Exec(`UPDATE site_quotes SET status = 'sent', sent_at = CURRENT_TIMESTAMP, credits = ?, price = ?, monthly = ?,
		sent_subject = ?, sent_body = ? WHERE id = ? AND status = 'draft'`, credits, price, monthly, subject, body, row.ID)
	if err != nil {
		http.Error(w, "not saved", http.StatusInternalServerError)
		return
	}
	if n, _ := res.RowsAffected(); n != 1 {
		renderQuotePage(w, http.StatusGone, quotePageData{quoteRow: quoteRow{ID: row.ID}, Message: "This link has already been used."})
		return
	}
	if err := m.deliver("EmpowerTours", row.Email, h.SMTP.To, subject, body); err != nil {
		log.Printf("quote #%d: client email failed: %v", row.ID, err)
		h.DB.Exec(`UPDATE site_quotes SET status = 'draft', sent_at = NULL WHERE id = ? AND status = 'sent'`, row.ID)
		renderQuotePage(w, http.StatusBadGateway, quotePageData{quoteRow: quoteRow{ID: row.ID}, Message: "The email did not go out. Nothing was sent; go back and try again."})
		return
	}
	renderQuotePage(w, http.StatusOK, quotePageData{quoteRow: quoteRow{ID: row.ID, Status: "sent"}, Message: "Sent to " + row.Email + ". This link no longer works."})
}

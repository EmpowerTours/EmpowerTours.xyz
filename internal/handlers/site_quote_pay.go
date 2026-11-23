package handlers

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
)

// siteAPIBase is the public origin of this API; every emailed link starts with it.
const siteAPIBase = "https://api.empowertours.xyz"

func acceptLink(id int64, token string) string {
	return siteAPIBase + "/api/v1/site/quotes/" + strconv.FormatInt(id, 10) + "/accept?t=" + token
}

func payLink(id int64, token string) string {
	return siteAPIBase + "/api/v1/site/admin/quotes/" + strconv.FormatInt(id, 10) + "/pay?t=" + token
}

func acceptLine(lang, link string) string {
	if lang == "es" {
		return "Para aceptar esta cotización y ver los datos de pago, abre este enlace:\n" + link
	}
	return "To accept this quote and see the payment details, open this link:\n" + link
}

// ---------- Beneficiary account ----------

// SitePay is the account clients pay accepted quotes into (SPEI or CoDi, MXN only).
type SitePay struct {
	CLABE, Name, Bank string
}

// NewSitePay validates the CLABE once at startup. An invalid CLABE is dropped, so clients
// are told "we'll email you payment details" instead of being shown a bad account. The
// warning never includes the value.
func NewSitePay(clabe, name, bank string) SitePay {
	clabe = strings.TrimSpace(clabe)
	if clabe != "" && !ValidCLABE(clabe) {
		log.Printf("warning: SITE_PAY_CLABE is not a valid CLABE (18 digits, control digit); payment details stay hidden until it is fixed")
		clabe = ""
	}
	if clabe != "" {
		if _, ok := CLABEReceptor(clabe); !ok {
			log.Printf("warning: SITE_PAY_CLABE bank prefix is not in the Banxico participant list; CEP verification is off, manual mark-paid only")
		}
	}
	return SitePay{CLABE: clabe, Name: strings.TrimSpace(name), Bank: strings.TrimSpace(bank)}
}

func (p SitePay) ready() bool { return p.CLABE != "" && p.Name != "" }

// ---------- Quote lookup by client or pay token ----------

type payRow struct {
	ID           int64  `db:"id"`
	Lang         string `db:"lang"`
	Currency     string `db:"currency"`
	Price        int    `db:"price"`
	Monthly      int    `db:"monthly"`
	Status       string `db:"status"`
	Subject      string `db:"sent_subject"`
	Body         string `db:"sent_body"`
	ClientHash   string `db:"client_token_hash"`
	PayHash      string `db:"pay_token_hash"`
	AcceptedAt   string `db:"accepted_at"`
	AcceptedName string `db:"accepted_name"`
	DocHash      string `db:"accepted_doc_sha256"`
	Reference    string `db:"pay_reference"`
	PaidCents    int64  `db:"paid_amount_cents"`
	CEPStatus    string `db:"cep_status"`
	Name         string `db:"name"`
	Email        string `db:"email"`
}

type tokenKind int

const (
	clientToken tokenKind = iota
	payToken
)

// payQuoteByToken works like quoteByToken: it always hashes and compares in constant
// time, against a dummy hash when the id is unknown or the token was never set.
func (h *SiteHandler) payQuoteByToken(idParam, token string, kind tokenKind) (*payRow, bool) {
	var row payRow
	id, err := strconv.ParseInt(idParam, 10, 64)
	found := err == nil && id > 0 && h.DB.Get(&row, `SELECT q.id, q.lang, q.currency, q.price, q.monthly, q.status,
		q.sent_subject, q.sent_body, q.client_token_hash, q.pay_token_hash, COALESCE(CAST(q.accepted_at AS TEXT), '') AS accepted_at,
		q.accepted_name, q.accepted_doc_sha256, q.pay_reference, q.paid_amount_cents, q.cep_status, i.name, i.email
		FROM site_quotes q JOIN site_inquiries i ON i.id = q.inquiry_id WHERE q.id = ?`, id) == nil
	stored := row.ClientHash
	if kind == payToken {
		stored = row.PayHash
	}
	if !found || len(stored) != 64 {
		stored = dummyTokenHash
	}
	match := subtle.ConstantTimeCompare([]byte(tokenHash(token)), []byte(stored)) == 1
	if !found || !match || token == "" {
		return nil, false
	}
	return &row, true
}

// quoteDocHash is the acceptance receipt: sha256 over the quote exactly as sent.
func quoteDocHash(id int64, currency string, price, monthly int, subject, body string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d|%s|%d|%d|%s|%s", id, currency, price, monthly, subject, body)))
	return hex.EncodeToString(sum[:])
}

func payReference(id int64) string { return fmt.Sprintf("ET-Q%04d", id) }

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return clip(host, 64)
}

func priceText(currency string, n int, lang string) string {
	s := money(currency, n)
	if currency == "MXN" {
		if lang == "es" {
			return s + " (IVA incluido)"
		}
		return s + " (IVA included)"
	}
	return s
}

func exactAmount(currency string, price int) string {
	return currency + " " + thousands(price) + ".00"
}

func tr(lang, en, es string) string {
	if lang == "es" {
		return es
	}
	return en
}

// ---------- Client accept page: GET/POST /api/v1/site/quotes/{id}/accept ----------

var acceptPageTmpl = template.Must(template.New("a").Parse(`<!doctype html>
<html lang="{{.Lang}}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex"><title>{{.T.Title}} #{{.ID}}</title>
<style>body{font:15px/1.5 system-ui,sans-serif;max-width:760px;margin:24px auto;padding:0 16px;color:#111;background:#fff}
pre{white-space:pre-wrap;background:#f4f4f6;padding:12px;border-radius:8px;word-break:break-word}label{display:block;margin-top:12px;font-weight:600}
input[type=text]{width:100%;box-sizing:border-box;font:inherit;padding:8px}.check{font-weight:400}.msg{font-weight:600}
table{border-collapse:collapse}td{padding:4px 12px 4px 0;vertical-align:top}button{margin-top:16px;padding:10px 16px;font:inherit}</style></head><body>
<h1>{{.T.Title}} #{{.ID}}</h1>
{{if .Message}}<p class="msg">{{.Message}}</p>{{end}}
{{if .ShowQuote}}
<h2>{{.Subject}}</h2>
<pre>{{.Body}}</pre>
<p><b>{{.T.Price}}:</b> {{.PriceText}}<br><b>{{.T.Monthly}}:</b> {{.MonthlyText}}</p>
{{end}}
{{if .Form}}
<form method="post">
<input type="hidden" name="t" value="{{.Token}}">
<label>{{.T.FullName}} <input type="text" name="name" maxlength="120" required autocomplete="name" value="{{.EnteredName}}"></label>
<label class="check"><input type="checkbox" name="accept" value="yes" required> {{.T.Checkbox}}</label>
<button>{{.T.Accept}}</button>
</form>
{{end}}
{{if .Receipt}}<h2>{{.T.Receipt}}</h2><p>{{.T.ReceiptText}}</p><pre>{{.Receipt}}</pre>{{end}}
{{if .ShowPay}}<h2>{{.T.PayTitle}}</h2>
{{if .Pay}}<table>
<tr><td>{{.T.Beneficiary}}</td><td><b>{{.Pay.Name}}</b></td></tr>
{{if .Pay.Bank}}<tr><td>{{.T.Bank}}</td><td><b>{{.Pay.Bank}}</b></td></tr>{{end}}
<tr><td>CLABE</td><td><b>{{.Pay.CLABE}}</b></td></tr>
<tr><td>{{.T.Amount}}</td><td><b>{{.Exact}}</b></td></tr>
<tr><td>{{.T.Concept}}</td><td><b>{{.Reference}}</b></td></tr>
</table>
<p>{{.T.PayNote}}</p>
{{else}}<p>{{.T.EmailDetails}}</p>{{end}}
{{end}}
</body></html>`))

type acceptTexts struct {
	Title, Price, Monthly, FullName, Checkbox, Accept, Receipt, ReceiptText string
	PayTitle, Beneficiary, Bank, Amount, Concept, PayNote, EmailDetails     string
}

func acceptTextsFor(lang, ref string) acceptTexts {
	if lang == "es" {
		return acceptTexts{Title: "Cotización", Price: "Precio", Monthly: "Hosting mensual", FullName: "Nombre completo",
			Checkbox: "Acepto esta cotización, sus criterios de aceptación y sus condiciones de reembolso", Accept: "Aceptar cotización",
			Receipt: "Comprobante de aceptación", ReceiptText: "Huella SHA-256 de la cotización que aceptaste. Guárdala:",
			PayTitle: "Datos de pago (SPEI o CoDi)", Beneficiary: "Beneficiario", Bank: "Banco", Amount: "Monto exacto", Concept: "Concepto",
			PayNote:      "Pon la referencia " + ref + " en el concepto de la transferencia y envía el monto exacto. Te confirmamos por correo cuando la recibamos.",
			EmailDetails: "Te enviaremos los datos de pago por correo."}
	}
	return acceptTexts{Title: "Quote", Price: "Price", Monthly: "Monthly hosting", FullName: "Full name",
		Checkbox: "I accept this quote, its acceptance checks and refund terms", Accept: "Accept quote",
		Receipt: "Acceptance receipt", ReceiptText: "SHA-256 fingerprint of the quote you accepted. Keep it:",
		PayTitle: "Payment details (SPEI or CoDi)", Beneficiary: "Beneficiary", Bank: "Bank", Amount: "Exact amount", Concept: "Concept",
		PayNote:      "Put reference " + ref + " in the transfer concept and send the exact amount. We email you when it arrives.",
		EmailDetails: "We'll email you payment details."}
}

type acceptPageData struct {
	ID                     int64
	Lang                   string
	T                      acceptTexts
	Message                string
	ShowQuote              bool
	Subject, Body          string
	PriceText, MonthlyText string
	Form                   bool
	Token, EnteredName     string
	Receipt                string
	ShowPay                bool
	Pay                    *SitePay
	Exact, Reference       string
}

func (h *SiteHandler) acceptData(row *payRow) acceptPageData {
	ref := row.Reference
	if ref == "" {
		ref = payReference(row.ID)
	}
	lang := tr(row.Lang, "en", "es")
	d := acceptPageData{ID: row.ID, Lang: lang, T: acceptTextsFor(lang, ref), ShowQuote: true, Subject: row.Subject, Body: row.Body,
		PriceText: priceText(row.Currency, row.Price, lang), MonthlyText: priceText(row.Currency, row.Monthly, lang),
		Exact: exactAmount(row.Currency, row.Price), Reference: ref}
	if h.Pay.ready() && row.Currency == "MXN" { // SPEI and CoDi move pesos only
		p := h.Pay
		d.Pay = &p
	}
	return d
}

func renderAcceptPage(w http.ResponseWriter, code int, d acceptPageData) {
	setQuotePageHeaders(w)
	w.WriteHeader(code)
	if err := acceptPageTmpl.Execute(w, d); err != nil {
		log.Printf("accept page: %v", err)
	}
}

// AcceptPage handles GET /api/v1/site/quotes/{id}/accept?t=… It only reads.
func (h *SiteHandler) AcceptPage(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("t")
	row, ok := h.payQuoteByToken(chi.URLParam(r, "id"), token, clientToken)
	if !ok {
		quoteNotFound(w)
		return
	}
	d := h.acceptData(row)
	switch row.Status {
	case "sent":
		if row.Price <= 0 {
			d.Message = tr(row.Lang, "This quote has no price yet, so it cannot be accepted. We'll send you an updated quote.",
				"Esta cotización aún no tiene precio, así que no se puede aceptar. Te enviaremos una cotización actualizada.")
		} else {
			d.Form, d.Token = true, token
		}
		renderAcceptPage(w, http.StatusOK, d)
	case "accepted":
		d.Message = tr(row.Lang, "You accepted this quote on "+row.AcceptedAt+" UTC.", "Aceptaste esta cotización el "+row.AcceptedAt+" UTC.")
		d.Receipt, d.ShowPay = row.DocHash, true
		renderAcceptPage(w, http.StatusOK, d)
	case "paid":
		d.Message = tr(row.Lang, "Payment received. Thank you!", "Pago recibido. ¡Gracias!")
		d.Receipt = row.DocHash
		renderAcceptPage(w, http.StatusOK, d)
	default:
		quoteNotFound(w)
	}
}

func cleanName(s string) string {
	s = oneLine(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s), 120)
	if utf8.RuneCountInString(s) < 2 {
		return ""
	}
	return s
}

// Accept handles POST /api/v1/site/quotes/{id}/accept (form: t, name, accept=yes).
// The sent -> accepted move is one conditional UPDATE, so a double POST accepts once.
func (h *SiteHandler) Accept(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	token := r.PostForm.Get("t")
	row, ok := h.payQuoteByToken(chi.URLParam(r, "id"), token, clientToken)
	if !ok {
		quoteNotFound(w)
		return
	}
	d := h.acceptData(row)
	already := func() {
		d.Message = tr(row.Lang, "This quote has already been accepted.", "Esta cotización ya fue aceptada.")
		renderAcceptPage(w, http.StatusConflict, d)
	}
	switch row.Status {
	case "sent":
	case "accepted", "paid":
		already()
		return
	default:
		quoteNotFound(w)
		return
	}
	if row.Price <= 0 {
		d.Message = tr(row.Lang, "This quote has no price yet, so it cannot be accepted.", "Esta cotización aún no tiene precio, así que no se puede aceptar.")
		renderAcceptPage(w, http.StatusBadRequest, d)
		return
	}
	name := cleanName(r.PostForm.Get("name"))
	if name == "" || r.PostForm.Get("accept") != "yes" {
		d.Form, d.Token, d.EnteredName = true, token, clipRunes(r.PostForm.Get("name"), 120)
		d.Message = tr(row.Lang, "Write your full name and tick the box to accept.", "Escribe tu nombre completo y marca la casilla para aceptar.")
		renderAcceptPage(w, http.StatusBadRequest, d)
		return
	}
	payTok, payHash, err := newApproveToken()
	if err != nil {
		http.Error(w, "not saved", http.StatusInternalServerError)
		return
	}
	doc := quoteDocHash(row.ID, row.Currency, row.Price, row.Monthly, row.Subject, row.Body)
	ref := payReference(row.ID)
	res, err := h.DB.Exec(`UPDATE site_quotes SET status = 'accepted', accepted_at = CURRENT_TIMESTAMP, accepted_name = ?, accepted_ip = ?,
		accepted_doc_sha256 = ?, pay_reference = ?, pay_token_hash = ? WHERE id = ? AND status = 'sent' AND client_token_hash = ?`,
		name, clientIP(r), doc, ref, payHash, row.ID, row.ClientHash)
	if err != nil {
		http.Error(w, "not saved", http.StatusInternalServerError)
		return
	}
	if n, _ := res.RowsAffected(); n != 1 {
		already()
		return
	}
	d.Message = tr(row.Lang, "Thank you, "+name+". Your acceptance is recorded.", "Gracias, "+name+". Registramos tu aceptación.")
	d.Receipt, d.ShowPay, d.Reference = doc, true, ref
	renderAcceptPage(w, http.StatusOK, d)

	m := h.mail()
	if !m.ready() {
		log.Printf("quote #%d accepted; mail not configured, no pay-verify link sent", row.ID)
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Quote #%d was accepted.\n\nAccepted by: %s\nClient: %s <%s>\nIP: %s\nAmount: %s\nReference: %s\nDocument SHA-256: %s\n\n",
		row.ID, name, row.Name, row.Email, clientIP(r), exactAmount(row.Currency, row.Price), ref, doc)
	if d.Pay == nil {
		fmt.Fprintf(&b, "The client was told you will email payment details (no SITE_PAY_CLABE, or the quote is not in MXN).\n\n")
	}
	fmt.Fprintf(&b, "When the transfer arrives, verify it against Banxico CEP (link works until the quote is marked paid):\n%s\n", payLink(row.ID, payTok))
	if err := m.deliver("EmpowerTours site", h.SMTP.To, row.Email, fmt.Sprintf("Quote #%d accepted", row.ID), b.String()); err != nil {
		log.Printf("quote #%d: accepted email: %v", row.ID, err)
	}
}

// ---------- Owner pay-verify page: GET/POST /api/v1/site/admin/quotes/{id}/pay ----------

type bankOption struct{ Code, Name string }

var bankOptions = func() []bankOption {
	out := make([]bankOption, 0, len(speiBankNames))
	for code, name := range speiBankNames {
		out = append(out, bankOption{code, name})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}()

var payPageTmpl = template.Must(template.New("p").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex"><title>Verify payment #{{.ID}}</title>
<style>body{font:15px/1.5 system-ui,sans-serif;max-width:760px;margin:24px auto;padding:0 16px;color:#111;background:#fff}
label{display:block;margin-top:12px;font-weight:600}input,select{width:100%;box-sizing:border-box;font:inherit;padding:8px}
input[type=checkbox]{width:auto}.check{font-weight:400}.msg{font-weight:600}fieldset{margin-top:20px;border:1px solid #ccc;border-radius:8px}
button{margin-top:16px;padding:10px 16px;font:inherit}</style></head><body>
<h1>Verify payment for quote #{{.ID}}</h1>
{{if .Message}}<p class="msg">{{.Message}}</p>{{end}}
{{if .Form}}
<p>{{.ClientName}} &lt;{{.ClientEmail}}&gt; · accepted by {{.AcceptedName}} on {{.AcceptedAt}} UTC<br>
Amount due: <b>{{.Exact}}</b> · reference <b>{{.Reference}}</b></p>
{{if .CanVerify}}
<form method="post">{{template "fields" .}}
<button name="action" value="verify">Verify with Banxico CEP</button>
</form>
{{else}}<p>{{.NoVerify}}</p>{{end}}
{{if .Manual}}
<fieldset><legend>Mark paid manually</legend>
<form method="post">{{template "fields" .}}
<label class="check"><input type="checkbox" name="confirm" value="yes" required> I checked my bank statement myself: this transfer arrived in full. Record it as paid without a CEP.</label>
<button name="action" value="manual">Mark paid manually</button>
</form></fieldset>
{{end}}
{{end}}
</body></html>
{{define "fields"}}
<input type="hidden" name="t" value="{{.Token}}">
<label>Fecha de operación <input type="date" name="fecha" value="{{.Fecha}}" required></label>
<label>Clave de rastreo{{if not .SPEI}} (optional){{end}} <input name="clave" maxlength="30" value="{{.Clave}}"{{if .SPEI}} required{{end}}></label>
<label>Banco emisor{{if not .SPEI}} (optional){{end}} <select name="emisor"{{if .SPEI}} required{{end}}><option value="">Choose…</option>{{$sel := .Emisor}}{{range .Banks}}<option value="{{.Code}}"{{if eq .Code $sel}} selected{{end}}>{{.Name}} ({{.Code}})</option>{{end}}</select></label>
<label>Amount ({{.Currency}}) <input name="amount" inputmode="decimal" value="{{.Amount}}" required></label>
{{end}}`))

type payPageData struct {
	ID                       int64
	Message                  string
	Form, CanVerify, Manual  bool
	NoVerify                 string
	ClientName, ClientEmail  string
	AcceptedName, AcceptedAt string
	Exact, Reference         string
	Currency                 string
	SPEI                     bool // MXN quote: tracking key and sending bank are required
	Token, Fecha, Clave      string
	Emisor, Amount           string
	Banks                    []bankOption
}

func renderPayPage(w http.ResponseWriter, code int, d payPageData) {
	setQuotePageHeaders(w)
	w.WriteHeader(code)
	if err := payPageTmpl.Execute(w, d); err != nil {
		log.Printf("pay page: %v", err)
	}
}

// verifyBlocker says why CEP verification cannot run for this quote, or "" when it can.
func (h *SiteHandler) verifyBlocker(row *payRow) string {
	if row.Currency != "MXN" {
		return "This quote is in " + row.Currency + "; SPEI/CEP verification covers MXN only. Record the payment manually once it has arrived."
	}
	if h.Pay.CLABE == "" {
		return "SITE_PAY_CLABE is not set (or invalid), so there is no account to verify against. Record the payment manually."
	}
	if _, ok := CLABEReceptor(h.Pay.CLABE); !ok {
		return "The SITE_PAY_CLABE bank is not in the Banxico participant list, so CEP cannot be queried. Record the payment manually."
	}
	return ""
}

func (h *SiteHandler) payData(row *payRow, token string) payPageData {
	d := payPageData{ID: row.ID, Form: true, ClientName: row.Name, ClientEmail: row.Email, AcceptedName: row.AcceptedName,
		AcceptedAt: row.AcceptedAt, Exact: exactAmount(row.Currency, row.Price), Reference: row.Reference, Token: token,
		Amount: strconv.Itoa(row.Price) + ".00", Banks: bankOptions, Currency: row.Currency, SPEI: row.Currency == "MXN"}
	d.NoVerify = h.verifyBlocker(row)
	d.CanVerify = d.NoVerify == ""
	d.Manual = !d.CanVerify
	return d
}

func payGone(w http.ResponseWriter, row *payRow) {
	msg := "This link has been used: quote #" + strconv.FormatInt(row.ID, 10) + " is " + row.Status + "."
	if row.Status == "paid" {
		msg = "Quote #" + strconv.FormatInt(row.ID, 10) + " is already recorded as paid (" + row.CEPStatus + "). This link no longer works."
	}
	renderPayPage(w, http.StatusGone, payPageData{ID: row.ID, Message: msg})
}

// PayPage handles GET /api/v1/site/admin/quotes/{id}/pay?t=… It only reads.
func (h *SiteHandler) PayPage(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("t")
	row, ok := h.payQuoteByToken(chi.URLParam(r, "id"), token, payToken)
	if !ok {
		quoteNotFound(w)
		return
	}
	if row.Status != "accepted" {
		payGone(w, row)
		return
	}
	renderPayPage(w, http.StatusOK, h.payData(row, token))
}

var reClaveRastreo = regexp.MustCompile(`^[A-Za-z0-9]{1,30}$`)

type payInput struct {
	fecha       time.Time
	clave       string
	emisor      string
	amountCents int64
}

// readPayInput validates the form. spei (MXN quotes) makes the tracking key and sending
// bank required; for other currencies they are optional but still checked when given.
func readPayInput(r *http.Request, spei bool) (payInput, error) {
	var in payInput
	f := strings.TrimSpace(r.PostForm.Get("fecha"))
	t, err := time.Parse("2006-01-02", f)
	if err != nil {
		if t, err = time.Parse("02-01-2006", f); err != nil {
			return in, errors.New("Fecha de operación must be a date (YYYY-MM-DD).")
		}
	}
	if t.After(time.Now().Add(36 * time.Hour)) {
		return in, errors.New("Fecha de operación is in the future.")
	}
	in.fecha = t
	in.clave = strings.TrimSpace(r.PostForm.Get("clave"))
	if (spei || in.clave != "") && !reClaveRastreo.MatchString(in.clave) {
		return in, errors.New("Clave de rastreo must be 1 to 30 letters or digits.")
	}
	in.emisor = r.PostForm.Get("emisor")
	if _, ok := speiBankNames[in.emisor]; (spei || in.emisor != "") && !ok {
		return in, errors.New("Choose the sending bank from the list.")
	}
	if in.amountCents, err = parseCents(r.PostForm.Get("amount")); err != nil || in.amountCents <= 0 {
		return in, errors.New("Amount must be a positive amount like 22950.00.")
	}
	return in, nil
}

// PayAction handles POST /api/v1/site/admin/quotes/{id}/pay (form: t, action=verify|manual,
// fecha, clave, emisor, amount, confirm). Failed attempts leave the link usable; the
// accepted -> paid move clears the token, so it works for one successful transition.
func (h *SiteHandler) PayAction(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	token := r.PostForm.Get("t")
	row, ok := h.payQuoteByToken(chi.URLParam(r, "id"), token, payToken)
	if !ok {
		quoteNotFound(w)
		return
	}
	if row.Status != "accepted" {
		payGone(w, row)
		return
	}
	d := h.payData(row, token)
	d.Fecha, d.Clave, d.Emisor, d.Amount = r.PostForm.Get("fecha"), clip(r.PostForm.Get("clave"), 30), r.PostForm.Get("emisor"), clip(r.PostForm.Get("amount"), 20)
	fail := func(code int, msg string, offerManual bool) {
		d.Message = msg
		d.Manual = d.Manual || offerManual
		renderPayPage(w, code, d)
	}
	in, err := readPayInput(r, row.Currency == "MXN")
	if err != nil {
		fail(http.StatusBadRequest, err.Error()+" Nothing was recorded.", false)
		return
	}
	due := int64(row.Price) * 100
	receptor, _ := CLABEReceptor(h.Pay.CLABE)

	switch r.PostForm.Get("action") {
	case "verify":
		if !d.CanVerify {
			fail(http.StatusBadRequest, d.NoVerify, true)
			return
		}
		res, err := h.CEP.Lookup(r.Context(), cepQuery{Fecha: in.fecha, ClaveRastreo: in.clave, Emisor: in.emisor, Receptor: receptor,
			Cuenta: h.Pay.CLABE, MontoCents: in.amountCents})
		switch {
		case errors.Is(err, errCEPNotFound):
			fail(http.StatusUnprocessableEntity, "Not found: "+err.Error()+". Check the fields against the client's receipt. Nothing was recorded.", false)
			return
		case err != nil:
			log.Printf("quote #%d: CEP lookup: %v", row.ID, err)
			reason := errCEPUnavailable.Error()
			for _, e := range []error{errCEPNotYet, errCEPRateLimited} {
				if errors.Is(err, e) {
					reason = e.Error()
				}
			}
			fail(http.StatusServiceUnavailable, "Not verified: "+reason+". Nothing was recorded. Retry later, or mark it paid manually below after checking your bank.", true)
			return
		}
		x := res.Parsed
		if x.Beneficiario.Cuenta != h.Pay.CLABE {
			fail(http.StatusUnprocessableEntity, "Refused: the CEP beneficiary account is not SITE_PAY_CLABE. This transfer did not go to our account. Nothing was recorded.", false)
			return
		}
		got, err := parseCents(x.Beneficiario.MontoPago)
		if err != nil || got != due {
			fail(http.StatusUnprocessableEntity, fmt.Sprintf("Refused: the CEP amount is MXN %s but the quote is %s. Nothing was recorded.",
				clip(x.Beneficiario.MontoPago, 20), exactAmount(row.Currency, row.Price)), false)
			return
		}
		if x.ClaveRastreo != "" && !strings.EqualFold(x.ClaveRastreo, in.clave) {
			fail(http.StatusUnprocessableEntity, "Refused: the CEP tracking key does not match the one entered. Nothing was recorded.", false)
			return
		}
		h.markPaid(w, row, in, receptor, got, res.Raw, "verified", d)
	case "manual":
		if r.PostForm.Get("confirm") != "yes" {
			fail(http.StatusBadRequest, "Tick the confirmation box to mark it paid manually. Nothing was recorded.", true)
			return
		}
		if in.amountCents != due {
			fail(http.StatusBadRequest, "The amount must equal the quote price ("+exactAmount(row.Currency, row.Price)+"); partial payments are not supported. Nothing was recorded.", true)
			return
		}
		h.markPaid(w, row, in, receptor, in.amountCents, "", "manual", d)
	default:
		http.Error(w, "unknown action", http.StatusBadRequest)
	}
}

func (h *SiteHandler) markPaid(w http.ResponseWriter, row *payRow, in payInput, receptor string, cents int64, cepXML, status string, d payPageData) {
	res, err := h.DB.Exec(`UPDATE site_quotes SET status = 'paid', paid_at = CURRENT_TIMESTAMP, paid_amount_cents = ?, cep_fecha = ?,
		cep_clave_rastreo = ?, cep_emisor = ?, cep_receptor = ?, cep_xml = ?, cep_status = ?, pay_token_hash = ''
		WHERE id = ? AND status = 'accepted' AND pay_token_hash = ?`,
		cents, in.fecha.Format("2006-01-02"), in.clave, in.emisor, receptor, cepXML, status, row.ID, row.PayHash)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			d.Message = "Refused: this transfer (date, tracking key, bank) is already recorded on another quote. Nothing was recorded."
			renderPayPage(w, http.StatusConflict, d)
			return
		}
		http.Error(w, "not saved", http.StatusInternalServerError)
		return
	}
	if n, _ := res.RowsAffected(); n != 1 {
		renderPayPage(w, http.StatusGone, payPageData{ID: row.ID, Message: "This link has already been used."})
		return
	}
	how := "verified against Banxico CEP"
	if status == "manual" {
		how = "recorded manually (no CEP)"
	}
	msg := fmt.Sprintf("Quote #%d marked paid: %s %s, %s. This link no longer works.", row.ID, row.Currency, centsToPesos(cents), how)
	m := h.mail()
	if !m.ready() {
		log.Printf("quote #%d paid; mail not configured, client not emailed", row.ID)
		msg += " Email is not configured, so the client was NOT notified."
	} else if err := m.deliver("EmpowerTours", row.Email, h.SMTP.To, paidSubject(row), paidBody(row, cents)); err != nil {
		log.Printf("quote #%d: paid email: %v", row.ID, err)
		msg += " The client email failed; tell them yourself."
	} else {
		msg += " The client was emailed."
	}
	renderPayPage(w, http.StatusOK, payPageData{ID: row.ID, Message: msg})
}

func paidSubject(row *payRow) string {
	return tr(row.Lang, fmt.Sprintf("Payment received for quote #%d", row.ID), fmt.Sprintf("Pago recibido: cotización #%d", row.ID))
}

func paidBody(row *payRow, cents int64) string {
	amount := row.Currency + " " + thousands(int(cents/100)) + fmt.Sprintf(".%02d", cents%100)
	if row.Lang == "es" {
		return fmt.Sprintf("Hola %s,\n\nRecibimos tu pago de %s para la cotización #%d (referencia %s). Empezamos a trabajar en tu proyecto y te escribimos pronto con los siguientes pasos.\n\nGracias,\nEmpowerTours",
			row.Name, amount, row.ID, row.Reference)
	}
	return fmt.Sprintf("Hi %s,\n\nWe received your payment of %s for quote #%d (reference %s). Work on your project starts now; we'll write soon with the next steps.\n\nThank you,\nEmpowerTours",
		row.Name, amount, row.ID, row.Reference)
}

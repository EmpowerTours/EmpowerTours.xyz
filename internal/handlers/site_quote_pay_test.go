package handlers

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
)

// A CLABE from cep-python's own tests (Cuenca, prefix 723 -> participant 90723).
const testCLABE = "723969000011000077"

// ---------- fake Banxico CEP portal ----------

type fakeBanxico struct {
	mu       sync.Mutex
	srv      *httptest.Server
	mode     string // found | notfound | notfound2 | notyet | ratelimited | ratelimited_xml | error | huge
	cuenta   string
	monto    string
	clave    string
	lastForm url.Values
	calls    int
}

func newFakeBanxico(t *testing.T) *fakeBanxico {
	f := &fakeBanxico{mode: "found", cuenta: testCLABE, monto: "22950.00", clave: "BBVA1234567890"}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.URL.Path {
		case "/cep/valida.do":
			f.calls++
			r.ParseForm()
			f.lastForm = r.PostForm
			http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "s1", Path: "/cep"})
			switch f.mode {
			case "notfound":
				fmt.Fprint(w, "<div>No se encontró ningún pago con la información proporcionada</div>")
			case "notfound2":
				fmt.Fprint(w, "<div>El SPEI no ha recibido una orden de pago que cumpla con el criterio de búsqueda especificado</div>")
			case "notyet":
				fmt.Fprint(w, "<div>Con la información proporcionada se identificó el siguiente pago</div>")
			case "ratelimited":
				fmt.Fprint(w, "<p>Lo sentimos, pero ha excedido el n&uacute;mero m&aacute;ximo de consultas en este portal</p>")
			case "error":
				http.Error(w, "boom", http.StatusInternalServerError)
			default:
				fmt.Fprint(w, `<a href="descarga.do?formato=XML">XML</a>`)
			}
		case "/cep/descarga.do":
			if c, err := r.Cookie("JSESSIONID"); err != nil || c.Value != "s1" || r.URL.Query().Get("formato") != "XML" {
				http.Error(w, "no session", http.StatusForbidden)
				return
			}
			switch f.mode {
			case "ratelimited_xml":
				fmt.Fprint(w, "<p>Lo sentimos, pero ha excedido el n&uacute;mero m&aacute;ximo de consultas en este portal</p>")
			case "huge":
				w.Write(bytes.Repeat([]byte("x"), cepMaxBody+10))
			default:
				fmt.Fprintf(w, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\r\n<SPEI_Tercero FechaOperacion=\"2024-11-08\" Hora=\"10:53:36\" ClaveSPEI=\"90723\" claveRastreo=\"%s\">"+
					"<Beneficiario BancoReceptor=\"Cuenca\" Nombre=\"EmpowerTours\" TipoCuenta=\"40\" Cuenta=\"%s\" RFC=\"NA\" Concepto=\"ET-Q0001\" IVA=\"0.00\" MontoPago=\"%s\"/>"+
					"<Ordenante BancoEmisor=\"BBVA MEXICO\" Nombre=\"Ana\" TipoCuenta=\"40\" Cuenta=\"012180004643051249\" RFC=\"NA\"/></SPEI_Tercero>", f.clave, f.cuenta, f.monto)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeBanxico) set(mode string) { f.mu.Lock(); f.mode = mode; f.mu.Unlock() }

// ---------- fixtures ----------

func payRouter(h *SiteHandler) http.Handler {
	r := chi.NewRouter()
	r.Get("/q/{id}", h.QuotePage)
	r.Post("/q/{id}", h.QuoteAction)
	r.Get("/a/{id}", h.AcceptPage)
	r.Post("/a/{id}", h.Accept)
	r.Get("/p/{id}", h.PayPage)
	r.Post("/p/{id}", h.PayAction)
	return r
}

var (
	reAcceptLink = regexp.MustCompile(`/api/v1/site/quotes/(\d+)/accept\?t=([A-Za-z0-9_-]+)`)
	rePayLink    = regexp.MustCompile(`/api/v1/site/admin/quotes/(\d+)/pay\?t=([A-Za-z0-9_-]+)`)
)

func newPayHandler(t *testing.T) (*SiteHandler, *stubMailer, *fakeBanxico) {
	f := newFakeBanxico(t)
	m := &stubMailer{}
	h := &SiteHandler{DB: siteTestDB(t), mailer: m, SMTP: SiteSMTP{To: "owner@example.com"},
		Pay: NewSitePay(testCLABE, "EmpowerTours SA de CV", "Cuenca"), CEP: &CEPClient{BaseURL: f.srv.URL + "/cep"}}
	return h, m, f
}

func lastMail(m *stubMailer) sentMail {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sent[len(m.sent)-1]
}

// sentQuote creates a quote and sends it at the given price; returns id and client token.
func sentQuote(t *testing.T, h *SiteHandler, m *stubMailer, price string) (string, string) {
	t.Helper()
	id, token := newQuote(t, h, m)
	form := url.Values{"t": {token}, "action": {"send"}, "credits": {"51"}, "price": {price}, "monthly": {"499"},
		"subject": {"Tu cotización"}, "body": {"Hola Ana,\r\nPrecio MXN 22,950."}}
	if w := postForm(payRouter(h), "/q/"+id, form); w.Code != 200 {
		t.Fatalf("send: %d %s", w.Code, w.Body)
	}
	mm := reAcceptLink.FindStringSubmatch(lastMail(m).body)
	if mm == nil || mm[1] != id {
		t.Fatalf("no accept link in client email: %q", lastMail(m).body)
	}
	return id, mm[2]
}

// acceptedQuote sends and accepts a quote; returns id and the owner's pay token.
func acceptedQuote(t *testing.T, h *SiteHandler, m *stubMailer) (string, string) {
	t.Helper()
	id, ct := sentQuote(t, h, m, "22950")
	if w := postForm(payRouter(h), "/a/"+id, url.Values{"t": {ct}, "name": {"Ana López"}, "accept": {"yes"}}); w.Code != 200 {
		t.Fatalf("accept: %d %s", w.Code, w.Body)
	}
	mm := rePayLink.FindStringSubmatch(lastMail(m).body)
	if mm == nil || mm[1] != id {
		t.Fatal("no pay link in owner email")
	}
	return id, mm[2]
}

func get(srv http.Handler, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func quoteStatus(t *testing.T, h *SiteHandler, id string) (status, cepStatus string) {
	t.Helper()
	if err := h.DB.QueryRow(`SELECT status, cep_status FROM site_quotes WHERE id = ?`, id).Scan(&status, &cepStatus); err != nil {
		t.Fatal(err)
	}
	return
}

func payForm(token, action string) url.Values {
	return url.Values{"t": {token}, "action": {action}, "fecha": {"2024-11-08"}, "clave": {"BBVA1234567890"}, "emisor": {"40012"}, "amount": {"22950.00"}}
}

// ---------- pure functions ----------

func TestValidCLABE(t *testing.T) {
	// Known-good values from the clabe and cep-python test suites.
	for _, c := range []string{"002000000000000008", "723123456682660854", testCLABE, "723010123456789019", "012180004643051249"} {
		if !ValidCLABE(c) {
			t.Errorf("%s rejected", c)
		}
	}
	for _, c := range []string{"002000000000000007", "72396900001100007", "7239690000110000770", "72396900001100007a", "", "723969000011000078"} {
		if ValidCLABE(c) {
			t.Errorf("%q accepted", c)
		}
	}
	if code, ok := CLABEReceptor(testCLABE); !ok || code != "90723" {
		t.Errorf("receptor %q", code)
	}
	if code, _ := CLABEReceptor("012180004643051249"); code != "40012" {
		t.Errorf("BBVA receptor %q", code)
	}
	if _, ok := CLABEReceptor("999000000000000000"); ok {
		t.Error("unknown prefix mapped")
	}
	for abm, code := range clabeBankCodes {
		if _, ok := speiBankNames[code]; !ok {
			t.Errorf("%s -> %s has no bank name", abm, code)
		}
	}
}

func TestNewSitePayDropsInvalidCLABEWithoutLoggingIt(t *testing.T) {
	var buf bytes.Buffer
	orig := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(orig)
	p := NewSitePay("002000000000000007", "N", "B")
	if p.CLABE != "" || p.ready() {
		t.Errorf("invalid CLABE kept: %+v", p)
	}
	if !strings.Contains(buf.String(), "SITE_PAY_CLABE") || strings.Contains(buf.String(), "002000000000000007") {
		t.Errorf("log: %q", buf.String())
	}
	if p := NewSitePay(" "+testCLABE+" ", "N", "B"); p.CLABE != testCLABE || !p.ready() {
		t.Errorf("valid CLABE: %+v", p)
	}
}

func TestQuoteDocHashStable(t *testing.T) {
	a := quoteDocHash(1, "MXN", 22950, 499, "Tu cotización", "Hola\nAna")
	sum := sha256.Sum256([]byte("1|MXN|22950|499|Tu cotización|Hola\nAna"))
	if a != hex.EncodeToString(sum[:]) || a != quoteDocHash(1, "MXN", 22950, 499, "Tu cotización", "Hola\nAna") {
		t.Fatalf("hash %s", a)
	}
	if a == quoteDocHash(1, "MXN", 22951, 499, "Tu cotización", "Hola\nAna") || a == quoteDocHash(1, "MXN", 22950, 499, "Tu cotización", "Hola\nAna.") {
		t.Fatal("hash ignores a field")
	}
}

func TestParseCents(t *testing.T) {
	good := map[string]int64{"22950": 2295000, "22,950.00": 2295000, "$22,950.5": 2295050, "3414.95": 341495, " 0.01 ": 1}
	for in, want := range good {
		if got, err := parseCents(in); err != nil || got != want {
			t.Errorf("%q: %d %v", in, got, err)
		}
	}
	for _, in := range []string{"", "abc", "1.234", "1.", "-5", "1e3", "12.3.4"} {
		if _, err := parseCents(in); err == nil {
			t.Errorf("%q accepted", in)
		}
	}
}

// ---------- send adds the accept link ----------

func TestSendAddsAcceptLinkButStoresPlainBody(t *testing.T) {
	h, m, _ := newPayHandler(t)
	id, _ := sentQuote(t, h, m, "22950")
	c := lastMail(m)
	if !strings.HasPrefix(c.body, "Hola Ana,\nPrecio MXN 22,950.\n\n") || !strings.Contains(c.body, "Para aceptar esta cotización") ||
		!strings.Contains(c.body, siteAPIBase+"/api/v1/site/quotes/"+id+"/accept?t=") {
		t.Errorf("client email body: %q", c.body)
	}
	var body, hash string
	h.DB.QueryRow(`SELECT sent_body, client_token_hash FROM site_quotes WHERE id = ?`, id).Scan(&body, &hash)
	if body != "Hola Ana,\nPrecio MXN 22,950." || len(hash) != 64 {
		t.Errorf("stored body %q hash len %d", body, len(hash))
	}
}

func TestSendFailureClearsClientToken(t *testing.T) {
	h, m, _ := newPayHandler(t)
	id, token := newQuote(t, h, m)
	m.fail = true
	form := url.Values{"t": {token}, "action": {"send"}, "credits": {"51"}, "price": {"22950"}, "monthly": {"499"}, "subject": {"s"}, "body": {"b"}}
	if w := postForm(payRouter(h), "/q/"+id, form); w.Code != http.StatusBadGateway {
		t.Fatalf("send: %d", w.Code)
	}
	var hash string
	h.DB.QueryRow(`SELECT client_token_hash FROM site_quotes WHERE id = ?`, id).Scan(&hash)
	if hash != "" {
		t.Error("client token survived a failed send")
	}
}

// ---------- client accept ----------

func TestAcceptGetHasNoSideEffects(t *testing.T) {
	h, m, _ := newPayHandler(t)
	id, ct := sentQuote(t, h, m, "22950")
	snapshot := func() string {
		var s string
		h.DB.QueryRow(`SELECT status || '|' || COALESCE(accepted_at,'') || '|' || accepted_doc_sha256 || '|' || pay_token_hash || '|' || client_token_hash FROM site_quotes WHERE id = ?`, id).Scan(&s)
		return s
	}
	before, mails := snapshot(), m.count()
	srv := payRouter(h)
	for i := 0; i < 3; i++ {
		w := get(srv, "/a/"+id+"?t="+ct)
		if w.Code != 200 {
			t.Fatalf("GET: %d", w.Code)
		}
		hdr := w.Header()
		if !strings.Contains(hdr.Get("Content-Security-Policy"), "default-src 'none'") || hdr.Get("X-Frame-Options") != "DENY" ||
			hdr.Get("Referrer-Policy") != "no-referrer" || hdr.Get("Cache-Control") != "no-store" {
			t.Errorf("headers: %v", hdr)
		}
		b := w.Body.String()
		if !strings.Contains(b, "Tu cotización") || !strings.Contains(b, "Precio MXN 22,950.") || !strings.Contains(b, "MXN 22,950 (IVA incluido)") ||
			!strings.Contains(b, `name="accept"`) || strings.Contains(b, "<script") || strings.Contains(b, testCLABE) {
			t.Errorf("page: %s", b)
		}
	}
	if snapshot() != before || m.count() != mails {
		t.Fatalf("GET changed state: %q -> %q", before, snapshot())
	}
	if w := get(srv, "/a/"+id+"?t="+strings.Repeat("A", 43)); w.Code != 404 {
		t.Errorf("wrong token: %d", w.Code)
	}
	if w := get(srv, "/a/"+id+"?t="); w.Code != 404 {
		t.Errorf("empty token: %d", w.Code)
	}
}

func TestAcceptRecordsOnceAndShowsPayment(t *testing.T) {
	h, m, _ := newPayHandler(t)
	id, ct := sentQuote(t, h, m, "22950")
	srv := payRouter(h)
	mails := m.count()

	// Missing checkbox or name: refused, nothing recorded.
	if w := postForm(srv, "/a/"+id, url.Values{"t": {ct}, "name": {"Ana López"}}); w.Code != 400 {
		t.Errorf("no checkbox: %d", w.Code)
	}
	if w := postForm(srv, "/a/"+id, url.Values{"t": {ct}, "name": {" "}, "accept": {"yes"}}); w.Code != 400 {
		t.Errorf("no name: %d", w.Code)
	}
	if s, _ := quoteStatus(t, h, id); s != "sent" {
		t.Fatalf("status %s", s)
	}

	r := httptest.NewRequest(http.MethodPost, "/a/"+id, strings.NewReader(url.Values{"t": {ct}, "name": {"Ana López"}, "accept": {"yes"}}.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.RemoteAddr = "203.0.113.7:4444"
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("accept: %d %s", w.Code, w.Body)
	}
	var status, name, ip, doc, ref, acceptedAt, subject, body string
	var price, monthly int
	h.DB.QueryRow(`SELECT status, accepted_name, accepted_ip, accepted_doc_sha256, pay_reference, COALESCE(accepted_at,''), sent_subject, sent_body, price, monthly
		FROM site_quotes WHERE id = ?`, id).Scan(&status, &name, &ip, &doc, &ref, &acceptedAt, &subject, &body, &price, &monthly)
	if status != "accepted" || name != "Ana López" || ip != "203.0.113.7" || acceptedAt == "" || ref != "ET-Q0001" {
		t.Errorf("row: %s %q %q %q %q", status, name, ip, acceptedAt, ref)
	}
	if doc != quoteDocHash(1, "MXN", price, monthly, subject, body) {
		t.Error("doc hash does not match the sent quote")
	}
	page := w.Body.String()
	for _, s := range []string{doc, testCLABE, "EmpowerTours SA de CV", "Cuenca", "MXN 22,950.00", "ET-Q0001"} {
		if !strings.Contains(page, s) {
			t.Errorf("accept page missing %q", s)
		}
	}
	if m.count() != mails+1 {
		t.Fatalf("owner emails: %d", m.count()-mails)
	}
	o := lastMail(m)
	if o.to != "owner@example.com" || o.subject != "Quote #1 accepted" || !rePayLink.MatchString(o.body) || !strings.Contains(o.body, doc) {
		t.Errorf("owner email: %+v", o)
	}

	// Second POST: 409, nothing changes, no email.
	if w := postForm(srv, "/a/"+id, url.Values{"t": {ct}, "name": {"Mallory"}, "accept": {"yes"}}); w.Code != http.StatusConflict {
		t.Errorf("second accept: %d", w.Code)
	}
	var name2 string
	h.DB.QueryRow(`SELECT accepted_name FROM site_quotes WHERE id = ?`, id).Scan(&name2)
	if name2 != "Ana López" || m.count() != mails+1 {
		t.Errorf("second accept changed state: %q, mails %d", name2, m.count()-mails)
	}
	// GET afterwards is a receipt: no form, hash and payment details shown.
	g := get(srv, "/a/"+id+"?t="+ct)
	if g.Code != 200 || strings.Contains(g.Body.String(), `name="accept"`) || !strings.Contains(g.Body.String(), doc) {
		t.Errorf("GET after accept: %d", g.Code)
	}
}

func TestAcceptIsAtomicUnderConcurrency(t *testing.T) {
	h, m, _ := newPayHandler(t)
	id, ct := sentQuote(t, h, m, "22950")
	srv := payRouter(h)
	mails := m.count()
	var wg sync.WaitGroup
	codes := make([]int, 8)
	for i := range codes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i] = postForm(srv, "/a/"+id, url.Values{"t": {ct}, "name": {fmt.Sprintf("Client %d", i)}, "accept": {"yes"}}).Code
		}(i)
	}
	wg.Wait()
	ok, conflict := 0, 0
	for _, c := range codes {
		switch c {
		case 200:
			ok++
		case http.StatusConflict, http.StatusGone:
			conflict++
		}
	}
	if ok != 1 || conflict != len(codes)-1 || m.count() != mails+1 {
		t.Fatalf("codes %v, owner mails %d", codes, m.count()-mails)
	}
}

func TestAcceptRefusesZeroPrice(t *testing.T) {
	h, m, _ := newPayHandler(t)
	id, ct := sentQuote(t, h, m, "0")
	srv := payRouter(h)
	if g := get(srv, "/a/"+id+"?t="+ct); g.Code != 200 || strings.Contains(g.Body.String(), `name="accept"`) {
		t.Errorf("GET zero price shows the form: %d", g.Code)
	}
	if w := postForm(srv, "/a/"+id, url.Values{"t": {ct}, "name": {"Ana López"}, "accept": {"yes"}}); w.Code != 400 {
		t.Errorf("POST zero price: %d", w.Code)
	}
	if s, _ := quoteStatus(t, h, id); s != "sent" {
		t.Errorf("zero price accepted: %s", s)
	}
}

func TestAcceptWithoutCLABEStillRecords(t *testing.T) {
	h, m, _ := newPayHandler(t)
	h.Pay = NewSitePay("", "", "")
	id, ct := sentQuote(t, h, m, "22950")
	w := postForm(payRouter(h), "/a/"+id, url.Values{"t": {ct}, "name": {"Ana López"}, "accept": {"yes"}})
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Te enviaremos los datos de pago por correo.") || strings.Contains(w.Body.String(), "CLABE") {
		t.Errorf("no-CLABE accept: %d %s", w.Code, w.Body)
	}
	if s, _ := quoteStatus(t, h, id); s != "accepted" {
		t.Errorf("status %s", s)
	}
}

// ---------- owner pay verification ----------

func TestPayVerifiedByCEP(t *testing.T) {
	h, m, f := newPayHandler(t)
	id, pt := acceptedQuote(t, h, m)
	srv := payRouter(h)

	before := snapshotPay(h, id)
	g := get(srv, "/p/"+id+"?t="+pt)
	if g.Code != 200 || !strings.Contains(g.Body.String(), `value="22950.00"`) || !strings.Contains(g.Body.String(), `value="40012"`) ||
		strings.Contains(g.Body.String(), "Mark paid manually") {
		t.Fatalf("pay GET: %d %s", g.Code, g.Body)
	}
	if g.Header().Get("Cache-Control") != "no-store" || snapshotPay(h, id) != before || f.calls != 0 {
		t.Error("pay GET had side effects or wrong headers")
	}
	mails := m.count()
	w := postForm(srv, "/p/"+id, payForm(pt, "verify"))
	if w.Code != 200 {
		t.Fatalf("verify: %d %s", w.Code, w.Body)
	}
	want := map[string]string{"tipoCriterio": "T", "captcha": "c", "tipoConsulta": "1", "fecha": "08-11-2024", "criterio": "BBVA1234567890",
		"emisor": "40012", "receptor": "90723", "cuenta": testCLABE, "monto": "22950.00", "receptorParticipante": "0"}
	for k, v := range want {
		if f.lastForm.Get(k) != v {
			t.Errorf("banxico form %s = %q, want %q", k, f.lastForm.Get(k), v)
		}
	}
	var status, cep, xml, fecha, clave, emisor, receptor, paidAt, tok string
	var cents int64
	h.DB.QueryRow(`SELECT status, cep_status, cep_xml, cep_fecha, cep_clave_rastreo, cep_emisor, cep_receptor, COALESCE(paid_at,''), paid_amount_cents, pay_token_hash
		FROM site_quotes WHERE id = ?`, id).Scan(&status, &cep, &xml, &fecha, &clave, &emisor, &receptor, &paidAt, &cents, &tok)
	if status != "paid" || cep != "verified" || !strings.Contains(xml, "<SPEI_Tercero") || fecha != "2024-11-08" || clave != "BBVA1234567890" ||
		emisor != "40012" || receptor != "90723" || paidAt == "" || cents != 2295000 || tok != "" {
		t.Errorf("paid row: %s %s %v %s %s %s %s %q %d %q", status, cep, len(xml) > 0, fecha, clave, emisor, receptor, paidAt, cents, tok)
	}
	if m.count() != mails+1 {
		t.Fatalf("client emails: %d", m.count()-mails)
	}
	c := lastMail(m)
	if c.to != "ana@example.com" || !strings.Contains(c.subject, "Pago recibido") || !strings.Contains(c.body, "MXN 22,950.00") || !strings.Contains(c.body, "ET-Q0001") {
		t.Errorf("client paid email: %+v", c)
	}
	// Single use: the link is dead after the paid transition.
	if w := postForm(srv, "/p/"+id, payForm(pt, "verify")); w.Code != 404 && w.Code != http.StatusGone {
		t.Errorf("second verify: %d", w.Code)
	}
	if w := get(srv, "/p/"+id+"?t="+pt); w.Code != 404 && w.Code != http.StatusGone {
		t.Errorf("GET after paid: %d", w.Code)
	}
	if m.count() != mails+1 || f.calls != 1 {
		t.Errorf("extra mails %d or Banxico calls %d", m.count()-mails-1, f.calls)
	}
}

func snapshotPay(h *SiteHandler, id string) string {
	var s string
	h.DB.QueryRow(`SELECT status || '|' || pay_token_hash || '|' || cep_status || '|' || paid_amount_cents FROM site_quotes WHERE id = ?`, id).Scan(&s)
	return s
}

func TestPayRefusals(t *testing.T) {
	cases := []struct {
		name, mode, cuenta, monto string
		code                      int
		manual                    bool
		says                      string
	}{
		{"amount mismatch", "found", testCLABE, "22000.00", 422, false, "CEP amount is MXN 22000.00"},
		{"account mismatch", "found", "012180004643051249", "22950.00", 422, false, "beneficiary account"},
		{"not found", "notfound", testCLABE, "22950.00", 422, false, "Not found"},
		{"not found (operation)", "notfound2", testCLABE, "22950.00", 422, false, "Not found"},
		{"rate limited", "ratelimited", testCLABE, "22950.00", 503, true, "maximum number of queries"},
		{"rate limited on download", "ratelimited_xml", testCLABE, "22950.00", 503, true, "maximum number of queries"},
		{"CEP not yet available", "notyet", testCLABE, "22950.00", 503, true, "not available yet"},
		{"server error", "error", testCLABE, "22950.00", 503, true, "could not be reached"},
		{"oversized response", "huge", testCLABE, "22950.00", 503, true, "could not be reached"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h, m, f := newPayHandler(t)
			id, pt := acceptedQuote(t, h, m)
			f.mu.Lock()
			f.mode, f.cuenta, f.monto = c.mode, c.cuenta, c.monto
			f.mu.Unlock()
			mails := m.count()
			w := postForm(payRouter(h), "/p/"+id, payForm(pt, "verify"))
			body := w.Body.String()
			if w.Code != c.code || !strings.Contains(body, c.says) || strings.Contains(body, "Mark paid manually") != c.manual {
				t.Errorf("%d (manual offered %v): %s", w.Code, strings.Contains(body, "Mark paid manually"), body)
			}
			if s, cep := quoteStatus(t, h, id); s != "accepted" || cep != "" || m.count() != mails {
				t.Errorf("state changed: %s %s", s, cep)
			}
			// A failed attempt keeps the link usable.
			if g := get(payRouter(h), "/p/"+id+"?t="+pt); g.Code != 200 {
				t.Errorf("link dead after failure: %d", g.Code)
			}
		})
	}
}

func TestPayUnreachable(t *testing.T) {
	h, m, f := newPayHandler(t)
	id, pt := acceptedQuote(t, h, m)
	f.srv.Close()
	w := postForm(payRouter(h), "/p/"+id, payForm(pt, "verify"))
	if w.Code != 503 || !strings.Contains(w.Body.String(), "Mark paid manually") {
		t.Errorf("unreachable: %d %s", w.Code, w.Body)
	}
	if s, _ := quoteStatus(t, h, id); s != "accepted" {
		t.Errorf("status %s", s)
	}
}

func TestPayManual(t *testing.T) {
	h, m, f := newPayHandler(t)
	id, pt := acceptedQuote(t, h, m)
	srv := payRouter(h)
	if w := postForm(srv, "/p/"+id, payForm(pt, "manual")); w.Code != 400 {
		t.Errorf("manual without confirm: %d", w.Code)
	}
	short := payForm(pt, "manual")
	short.Set("confirm", "yes")
	short.Set("amount", "20000")
	if w := postForm(srv, "/p/"+id, short); w.Code != 400 {
		t.Errorf("manual partial amount: %d", w.Code)
	}
	if s, _ := quoteStatus(t, h, id); s != "accepted" {
		t.Fatalf("status %s", s)
	}
	ok := payForm(pt, "manual")
	ok.Set("confirm", "yes")
	mails := m.count()
	if w := postForm(srv, "/p/"+id, ok); w.Code != 200 {
		t.Fatalf("manual: %d %s", w.Code, w.Body)
	}
	if s, cep := quoteStatus(t, h, id); s != "paid" || cep != "manual" {
		t.Errorf("after manual: %s %s", s, cep)
	}
	if f.calls != 0 || m.count() != mails+1 {
		t.Errorf("manual called Banxico %d times, mails %d", f.calls, m.count()-mails)
	}
	if w := postForm(srv, "/p/"+id, ok); w.Code != 404 && w.Code != http.StatusGone {
		t.Errorf("second manual: %d", w.Code)
	}
}

func TestPayValidationAndTokens(t *testing.T) {
	h, m, f := newPayHandler(t)
	id, pt := acceptedQuote(t, h, m)
	srv := payRouter(h)
	for field, val := range map[string]string{"fecha": "yesterday", "clave": "BBVA-123 ", "emisor": "99999", "amount": "0"} {
		v := payForm(pt, "verify")
		v.Set(field, val)
		if w := postForm(srv, "/p/"+id, v); w.Code != 400 {
			t.Errorf("bad %s: %d", field, w.Code)
		}
	}
	// The client token does not open the pay page, and a wrong token is a 404.
	bad := payForm(strings.Repeat("A", 43), "verify")
	if w := postForm(srv, "/p/"+id, bad); w.Code != 404 {
		t.Errorf("wrong pay token: %d", w.Code)
	}
	if f.calls != 0 {
		t.Errorf("Banxico called %d times on invalid input", f.calls)
	}
	if _, ok := h.payQuoteByToken("999", pt, payToken); ok {
		t.Error("unknown id accepted")
	}
	if _, ok := h.payQuoteByToken(id, "", clientToken); ok {
		t.Error("empty token accepted")
	}
	if _, ok := h.payQuoteByToken(id, pt, clientToken); ok {
		t.Error("pay token opened the client page")
	}
}

func TestPaySameTransferTwiceRefused(t *testing.T) {
	h, m, _ := newPayHandler(t)
	id1, pt1 := acceptedQuote(t, h, m)
	id2, pt2 := acceptedQuote(t, h, m)
	srv := payRouter(h)
	if w := postForm(srv, "/p/"+id1, payForm(pt1, "verify")); w.Code != 200 {
		t.Fatalf("first: %d", w.Code)
	}
	if w := postForm(srv, "/p/"+id2, payForm(pt2, "verify")); w.Code != http.StatusConflict {
		t.Errorf("reused transfer: %d %s", w.Code, w.Body)
	}
	if s, _ := quoteStatus(t, h, id2); s != "accepted" {
		t.Errorf("second quote marked %s", s)
	}
}

func TestPayNonMXNIsManualOnly(t *testing.T) {
	h, m, f := newPayHandler(t)
	id, pt := acceptedQuote(t, h, m)
	h.DB.MustExec(`UPDATE site_quotes SET currency = 'USD' WHERE id = ?`, id)
	srv := payRouter(h)
	g := get(srv, "/p/"+id+"?t="+pt)
	if g.Code != 200 || strings.Contains(g.Body.String(), "Verify with Banxico") || !strings.Contains(g.Body.String(), "Mark paid manually") {
		t.Errorf("USD pay page: %s", g.Body)
	}
	if w := postForm(srv, "/p/"+id, payForm(pt, "verify")); w.Code != 400 || f.calls != 0 {
		t.Errorf("USD verify: %d, calls %d", w.Code, f.calls)
	}
	v := url.Values{"t": {pt}, "action": {"manual"}, "fecha": {"2024-11-08"}, "amount": {"22950"}, "confirm": {"yes"}}
	if w := postForm(srv, "/p/"+id, v); w.Code != 200 {
		t.Errorf("USD manual: %d %s", w.Code, w.Body)
	}
}

func TestRedactQuoteTokenCoversNewLinks(t *testing.T) {
	for _, p := range []string{"/api/v1/site/quotes/7/accept?t=SECRET", "/api/v1/site/admin/quotes/7/pay?t=SECRET"} {
		var seenURI, seenToken string
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			seenURI, seenToken = r.RequestURI, r.URL.Query().Get("t")
		})
		RedactQuoteToken(next).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, p, nil))
		if strings.Contains(seenURI, "SECRET") || seenToken != "SECRET" {
			t.Errorf("%s: uri %q token %q", p, seenURI, seenToken)
		}
	}
}

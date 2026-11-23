package handlers

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ---------- CLABE ----------

var clabeWeights = [17]int{3, 7, 1, 3, 7, 1, 3, 7, 1, 3, 7, 1, 3, 7, 1, 3, 7}

// ValidCLABE checks length, digits and the control digit (weights 3,7,1, as in the
// clabe package's compute_control_digit). It does not check the bank prefix.
func ValidCLABE(c string) bool {
	if len(c) != 18 {
		return false
	}
	sum := 0
	for i := 0; i < 18; i++ {
		if c[i] < '0' || c[i] > '9' {
			return false
		}
		if i < 17 {
			sum += int(c[i]-'0') * clabeWeights[i] % 10
		}
	}
	return int(c[17]-'0') == (10-sum%10)%10
}

// CLABEReceptor returns the Banxico participant code for the bank that holds the CLABE.
func CLABEReceptor(c string) (string, bool) {
	if len(c) < 3 {
		return "", false
	}
	code, ok := clabeBankCodes[c[:3]]
	return code, ok
}

// ---------- Banxico CEP client ----------
//
// Mirrors cuenca-mx/cep-python (cep/transferencia.py): POST /valida.do with the search
// criteria, then GET /descarga.do?formato=XML in the same cookie session.

const (
	cepDefaultBase = "https://www.banxico.org.mx/cep"
	cepTimeout     = 15 * time.Second
	cepMaxBody     = 1 << 20
	cepUserAgent   = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_14_6) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/75.0.3770.142 Safari/537.36"
)

var (
	errCEPNotFound    = errors.New("Banxico found no SPEI payment matching that date, tracking key, banks, account and amount")
	errCEPNotYet      = errors.New("Banxico found the payment but its CEP is not available yet; try again later")
	errCEPRateLimited = errors.New("Banxico says the maximum number of queries has been exceeded; try again later")
	errCEPUnavailable = errors.New("Banxico could not be reached or returned an unexpected response")
)

var (
	cepNotFoundTexts = []string{
		"No se encontró ningún pago con la información proporcionada",
		"El SPEI no ha recibido una orden de pago que cumpla con el criterio de búsqueda especificado",
	}
	cepNotYetText      = "Con la información proporcionada se identificó el siguiente pago"
	cepRateLimitedText = []string{"ha excedido el n&uacute;mero m&aacute;ximo de consultas", "ha excedido el número máximo de consultas"}
)

// CEPClient queries Banxico's CEP portal. The zero value talks to the real portal.
type CEPClient struct {
	BaseURL string // tests point this at an httptest server
	HTTP    *http.Client
}

type cepQuery struct {
	Fecha        time.Time
	ClaveRastreo string
	Emisor       string
	Receptor     string
	Cuenta       string
	MontoCents   int64
}

type cepAccount struct {
	Nombre    string `xml:"Nombre,attr"`
	Cuenta    string `xml:"Cuenta,attr"`
	MontoPago string `xml:"MontoPago,attr"`
	Concepto  string `xml:"Concepto,attr"`
}

type cepXML struct {
	XMLName        xml.Name   `xml:"SPEI_Tercero"`
	FechaOperacion string     `xml:"FechaOperacion,attr"`
	ClaveRastreo   string     `xml:"claveRastreo,attr"`
	Beneficiario   cepAccount `xml:"Beneficiario"`
	Ordenante      cepAccount `xml:"Ordenante"`
}

type cepResult struct {
	Raw    string
	Parsed cepXML
}

func (c *CEPClient) base() string {
	if c != nil && c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return cepDefaultBase
}

func (c *CEPClient) client() (*http.Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	var transport http.RoundTripper
	if c != nil && c.HTTP != nil {
		transport = c.HTTP.Transport
	}
	// A fresh jar per lookup: each lookup is its own Banxico session.
	return &http.Client{Timeout: cepTimeout, Jar: jar, Transport: transport}, nil
}

func cepRead(res *http.Response) (string, error) {
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, cepMaxBody+1))
	if err != nil {
		return "", err
	}
	if len(b) > cepMaxBody {
		return "", errors.New("response too large")
	}
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", res.StatusCode)
	}
	return string(b), nil
}

func centsToPesos(c int64) string { return fmt.Sprintf("%d.%02d", c/100, c%100) }

func cepRateLimited(s string) bool {
	for _, t := range cepRateLimitedText {
		if strings.Contains(s, t) {
			return true
		}
	}
	return false
}

// Lookup returns the CEP XML for one transfer, or one of the errCEP* errors.
func (c *CEPClient) Lookup(ctx context.Context, q cepQuery) (*cepResult, error) {
	hc, err := c.client()
	if err != nil {
		return nil, errCEPUnavailable
	}
	form := url.Values{
		"tipoCriterio":         {"T"},
		"captcha":              {"c"},
		"tipoConsulta":         {"1"},
		"fecha":                {q.Fecha.Format("02-01-2006")},
		"criterio":             {q.ClaveRastreo},
		"emisor":               {q.Emisor},
		"receptor":             {q.Receptor},
		"cuenta":               {q.Cuenta},
		"monto":                {centsToPesos(q.MontoCents)},
		"receptorParticipante": {"0"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base()+"/valida.do", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, errCEPUnavailable
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", cepUserAgent)
	res, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w (%v)", errCEPUnavailable, err)
	}
	page, err := cepRead(res)
	if err != nil {
		return nil, fmt.Errorf("%w (valida: %v)", errCEPUnavailable, err)
	}
	if cepRateLimited(page) {
		return nil, errCEPRateLimited
	}
	if strings.Contains(page, cepNotYetText) {
		return nil, errCEPNotYet
	}
	for _, t := range cepNotFoundTexts {
		if strings.Contains(page, t) {
			return nil, errCEPNotFound
		}
	}

	req, err = http.NewRequestWithContext(ctx, http.MethodGet, c.base()+"/descarga.do?formato=XML", nil)
	if err != nil {
		return nil, errCEPUnavailable
	}
	req.Header.Set("User-Agent", cepUserAgent)
	res, err = hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w (%v)", errCEPUnavailable, err)
	}
	raw, err := cepRead(res)
	if err != nil {
		return nil, fmt.Errorf("%w (descarga: %v)", errCEPUnavailable, err)
	}
	if cepRateLimited(raw) {
		return nil, errCEPRateLimited
	}
	var x cepXML
	if err := xml.Unmarshal([]byte(raw), &x); err != nil {
		return nil, fmt.Errorf("%w (CEP XML unreadable)", errCEPUnavailable)
	}
	return &cepResult{Raw: raw, Parsed: x}, nil
}

// parseCents reads a peso amount such as "3414.95", "22,950" or "$22,950.00" exactly.
func parseCents(s string) (int64, error) {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "$"))
	s = strings.ReplaceAll(s, ",", "")
	whole, frac, hasFrac := strings.Cut(s, ".")
	if whole == "" || len(whole) > 12 || (hasFrac && (len(frac) < 1 || len(frac) > 2)) {
		return 0, errors.New("amount must look like 22950 or 22950.00")
	}
	for _, r := range whole + frac {
		if r < '0' || r > '9' {
			return 0, errors.New("amount must look like 22950 or 22950.00")
		}
	}
	w, _ := strconv.ParseInt(whole, 10, 64)
	var f int64
	if hasFrac {
		if len(frac) == 1 {
			frac += "0"
		}
		f, _ = strconv.ParseInt(frac, 10, 64)
	}
	return w*100 + f, nil
}

package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// HireMON takes payment for a merged task in MON sent straight to our wallet
// (no escrow: we never hold client funds before the work is accepted).
//
// A payment intent locks the MXN->MON rate for 30 minutes and an exact amount
// whose last 4 decimals are unique among open intents, so one on-chain transfer
// can only match one task. The client sends exactly that amount and pastes the
// transaction hash; we check it on chain before marking the task paid.
type HireMON struct {
	RPCURL   string // Monad mainnet RPC
	Treasury string // 0x... our receiving wallet; empty disables MON payments
	ChainID  int64  // 143
	RateURL  string // price source returning {"monad":{"mxn":<float>}}
	MinConf  int64  // blocks on top of the payment block
}

const hireIntentTTL = 30 * time.Minute

var reAddr = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)
var reTxHash = regexp.MustCompile(`^0x[0-9a-f]{64}$`)

func (m HireMON) Enabled() bool {
	return m.RPCURL != "" && reAddr.MatchString(m.Treasury) && m.ChainID == 143
}

type monIntent struct {
	ID        int64     `db:"id" json:"id"`
	AmountWei string    `db:"amount_wei" json:"amount_wei"`
	Rate      string    `db:"rate_mxn_per_mon" json:"rate_mxn_per_mon"`
	PayTo     string    `db:"pay_to" json:"pay_to"`
	AmountMXN int64     `db:"amount_mxn" json:"amount_mxn"`
	ExpiresAt time.Time `db:"expires_at" json:"expires_at"`
	CreatedAt time.Time `db:"created_at" json:"-"`
	Amount    string    `db:"-" json:"amount_mon"`
}

// PayMONIntent returns the open intent for a merged task, or creates one.
func (h *HireHandler) PayMONIntent(w http.ResponseWriter, r *http.Request) {
	if !h.MON.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "MON payments are not open yet")
		return
	}
	t, err := h.ownedTask(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	if t.Stage != StageMerged || t.PriceMXN == nil || *t.PriceMXN <= 0 {
		writeError(w, http.StatusConflict, "a task is paid after you merge its pull request")
		return
	}
	ctx := r.Context()
	var in monIntent
	err = h.DB.GetContext(ctx, &in, `SELECT id, amount_wei, rate_mxn_per_mon, pay_to, amount_mxn, expires_at, created_at FROM hire_payments
		WHERE task_id = ? AND method = 'mon' AND status = 'pending' AND expires_at > ? ORDER BY id DESC LIMIT 1`, t.ID, h.now())
	if err == nil {
		in.Amount = weiToMON(in.AmountWei)
		writeJSON(w, http.StatusOK, in)
		return
	}
	rate, err := h.monRate(ctx)
	if err != nil {
		log.Printf("hire mon rate: %v", err)
		writeError(w, http.StatusServiceUnavailable, "MON price unavailable; try again in a minute")
		return
	}
	base := monForMXN(*t.PriceMXN, rate) // whole MON, rounded up
	for tries := 0; tries < 20; tries++ {
		tag, err := randomTag()
		if err != nil {
			break
		}
		wei := new(big.Int).Mul(base, big.NewInt(1e18))
		wei.Add(wei, new(big.Int).Mul(big.NewInt(tag), big.NewInt(1e14))) // .0001..0.9999 MON tag
		now := h.now()
		res, err := h.DB.ExecContext(ctx, `INSERT INTO hire_payments (task_id, method, amount_mxn, amount_wei, rate_mxn_per_mon, pay_to, status, expires_at, created_at)
			SELECT ?, 'mon', ?, ?, ?, ?, 'pending', ?, ? WHERE NOT EXISTS (
				SELECT 1 FROM hire_payments WHERE method = 'mon' AND amount_wei = ? AND status IN ('pending','confirmed') AND (status = 'confirmed' OR expires_at > ?))`,
			t.ID, *t.PriceMXN, wei.String(), strconv.FormatFloat(rate, 'f', -1, 64), strings.ToLower(h.MON.Treasury), now.Add(hireIntentTTL), now, wei.String(), now)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "try again")
			return
		}
		if n, _ := res.RowsAffected(); n == 1 {
			id, _ := res.LastInsertId()
			writeJSON(w, http.StatusOK, monIntent{ID: id, AmountWei: wei.String(), Rate: strconv.FormatFloat(rate, 'f', -1, 64),
				PayTo: strings.ToLower(h.MON.Treasury), AmountMXN: *t.PriceMXN, ExpiresAt: now.Add(hireIntentTTL), Amount: weiToMON(wei.String())})
			return
		}
	}
	writeError(w, http.StatusServiceUnavailable, "try again")
}

func randomTag() (int64, error) {
	tok, err := randomToken()
	if err != nil {
		return 0, err
	}
	n, _ := strconv.ParseInt(tok[:8], 16, 64)
	return n%9999 + 1, nil
}

// monForMXN converts pesos to whole MON at rate (MXN per MON), rounding up.
func monForMXN(mxn int64, rate float64) *big.Int {
	r := new(big.Rat).SetFloat64(rate)
	q := new(big.Rat).Quo(new(big.Rat).SetInt64(mxn), r)
	n := new(big.Int).Quo(q.Num(), q.Denom())
	if new(big.Rat).SetInt(n).Cmp(q) < 0 {
		n.Add(n, big.NewInt(1))
	}
	return n
}

func weiToMON(wei string) string {
	n, ok := new(big.Int).SetString(wei, 10)
	if !ok {
		return ""
	}
	whole, frac := new(big.Int).QuoRem(n, big.NewInt(1e18), new(big.Int))
	f := strings.TrimRight(fmt.Sprintf("%018s", frac.String()), "0")
	if f == "" {
		return whole.String()
	}
	return whole.String() + "." + f
}

func (h *HireHandler) monRate(ctx context.Context) (float64, error) {
	u := h.MON.RateURL
	if u == "" {
		u = "https://api.coingecko.com/api/v3/simple/price?ids=monad&vs_currencies=mxn"
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := h.client().Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	var out struct {
		Monad struct {
			MXN float64 `json:"mxn"`
		} `json:"monad"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&out); err != nil {
		return 0, err
	}
	if out.Monad.MXN <= 0 || out.Monad.MXN > 1e6 {
		return 0, fmt.Errorf("implausible rate %v", out.Monad.MXN)
	}
	return out.Monad.MXN, nil
}

// PayMONConfirm checks the client's transaction on chain and marks the task paid.
func (h *HireHandler) PayMONConfirm(w http.ResponseWriter, r *http.Request) {
	if !h.MON.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "MON payments are not open yet")
		return
	}
	t, err := h.ownedTask(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	var in struct {
		TxHash string `json:"tx_hash"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&in) != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	hash := strings.ToLower(strings.TrimSpace(in.TxHash))
	if !reTxHash.MatchString(hash) {
		writeError(w, http.StatusBadRequest, "paste the 0x… transaction hash")
		return
	}
	if t.Stage != StageMerged {
		writeError(w, http.StatusConflict, "this task is not waiting for payment")
		return
	}
	ctx := r.Context()
	var used int
	h.DB.GetContext(ctx, &used, `SELECT COUNT(*) FROM hire_payments WHERE tx_hash = ?`, hash)
	if used > 0 {
		writeError(w, http.StatusConflict, "this transaction was already used for a payment")
		return
	}
	var intents []monIntent
	h.DB.SelectContext(ctx, &intents, `SELECT id, amount_wei, rate_mxn_per_mon, pay_to, amount_mxn, expires_at, created_at FROM hire_payments
		WHERE task_id = ? AND method = 'mon' AND status = 'pending' ORDER BY id DESC LIMIT 10`, t.ID)
	if len(intents) == 0 {
		writeError(w, http.StatusConflict, "get a payment amount first")
		return
	}
	tx, err := h.fetchMONTransfer(ctx, hash)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var match *monIntent
	for i := range intents {
		in := intents[i]
		// The transfer must hit our wallet with the intent's exact amount, inside
		// the intent's window (2 minutes of slack for clock and block time).
		if tx.to == in.PayTo && tx.value == in.AmountWei &&
			!tx.at.Before(in.CreatedAt.Add(-2*time.Minute)) && !tx.at.After(in.ExpiresAt.Add(2*time.Minute)) {
			match = &in
			break
		}
	}
	if match == nil {
		writeError(w, http.StatusBadRequest, "that transaction doesn't match this task's amount, wallet or time window")
		return
	}
	dbtx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "try again")
		return
	}
	defer dbtx.Rollback()
	res, err := dbtx.ExecContext(ctx, `UPDATE hire_payments SET status = 'confirmed', tx_hash = ?, confirmed_at = ? WHERE id = ? AND status = 'pending'`, hash, h.now(), match.ID)
	if err != nil {
		writeError(w, http.StatusConflict, "this transaction was already used for a payment")
		return
	}
	if n, _ := res.RowsAffected(); n != 1 {
		writeError(w, http.StatusConflict, "already confirmed")
		return
	}
	ok, err := h.moveTask(ctx, dbtx, t.ID, StageMerged, StagePaid, `, paid_at = ?`, h.now())
	if err != nil || !ok {
		writeError(w, http.StatusConflict, "this task is not waiting for payment")
		return
	}
	dbtx.ExecContext(ctx, `UPDATE hire_payments SET status = 'expired' WHERE task_id = ? AND status = 'pending'`, t.ID)
	h.event(ctx, dbtx, t.ProjectID, &t.ID, "paid", "Task "+strconv.FormatInt(t.ID, 10)+" paid: "+weiToMON(match.AmountWei)+" MON")
	if err := dbtx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "try again")
		return
	}
	h.notify("Task paid in MON: "+t.Title, fmt.Sprintf("%s paid task %d: %s MON (%s at %s MXN/MON)\nTx: %s",
		clientFrom(r).Login, t.ID, weiToMON(match.AmountWei), mxnText(match.AmountMXN), match.Rate, hash))
	writeJSON(w, http.StatusOK, map[string]string{"stage": StagePaid})
}

type monTransfer struct {
	to, value string
	at        time.Time
}

// fetchMONTransfer reads a transaction and its receipt and block from the RPC,
// and returns it only if it is a successful plain transfer on chain 143 with
// enough confirmations.
func (h *HireHandler) fetchMONTransfer(ctx context.Context, hash string) (*monTransfer, error) {
	var txr struct {
		To          *string `json:"to"`
		Value       string  `json:"value"`
		BlockNumber *string `json:"blockNumber"`
		ChainID     string  `json:"chainId"`
	}
	if err := h.rpc(ctx, "eth_getTransactionByHash", []any{hash}, &txr); err != nil {
		return nil, errors.New("couldn't read that transaction; try again shortly")
	}
	if txr.BlockNumber == nil || txr.To == nil {
		return nil, errors.New("transaction not found or not mined yet")
	}
	if cid, ok := hexBig(txr.ChainID); ok && cid.Int64() != h.MON.ChainID {
		return nil, errors.New("that transaction is not on Monad mainnet")
	}
	var rc struct {
		Status string `json:"status"`
	}
	if err := h.rpc(ctx, "eth_getTransactionReceipt", []any{hash}, &rc); err != nil || rc.Status != "0x1" {
		return nil, errors.New("that transaction failed or isn't final yet")
	}
	var head string
	if err := h.rpc(ctx, "eth_blockNumber", []any{}, &head); err != nil {
		return nil, errors.New("couldn't read the chain; try again shortly")
	}
	hb, ok1 := hexBig(head)
	bb, ok2 := hexBig(*txr.BlockNumber)
	if !ok1 || !ok2 || new(big.Int).Sub(hb, bb).Int64() < h.MON.MinConf {
		return nil, errors.New("waiting for confirmations; try again in a few seconds")
	}
	var blk struct {
		Timestamp string `json:"timestamp"`
	}
	if err := h.rpc(ctx, "eth_getBlockByNumber", []any{*txr.BlockNumber, false}, &blk); err != nil {
		return nil, errors.New("couldn't read the block; try again shortly")
	}
	ts, ok := hexBig(blk.Timestamp)
	val, ok3 := hexBig(txr.Value)
	if !ok || !ok3 {
		return nil, errors.New("unexpected transaction data")
	}
	return &monTransfer{to: strings.ToLower(*txr.To), value: val.String(), at: time.Unix(ts.Int64(), 0).UTC()}, nil
}

func hexBig(s string) (*big.Int, bool) {
	if !strings.HasPrefix(s, "0x") {
		return nil, false
	}
	return new(big.Int).SetString(s[2:], 16)
}

func (h *HireHandler) rpc(ctx context.Context, method string, params []any, out any) error {
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, h.MON.RPCURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var env struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&env); err != nil {
		return err
	}
	if env.Error != nil {
		return errors.New(env.Error.Message)
	}
	if len(env.Result) == 0 || string(env.Result) == "null" {
		return sql.ErrNoRows
	}
	return json.Unmarshal(env.Result, out)
}

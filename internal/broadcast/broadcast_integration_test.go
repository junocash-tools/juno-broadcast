//go:build integration

package broadcast_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Abdullah1738/juno-broadcast/internal/broadcast"
	"github.com/Abdullah1738/juno-broadcast/internal/testutil/containers"
	"github.com/Abdullah1738/juno-sdk-go/junocashd"
)

func TestClient_SubmitAndConfirm_Integration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	jd, err := containers.StartJunocashd(ctx)
	if err != nil {
		t.Fatalf("StartJunocashd: %v", err)
	}
	defer func() { _ = jd.Terminate(context.Background()) }()

	mustRunCLI(t, ctx, jd, "generate", "101")

	addr := mustCreateOrchardAddress(t, ctx, jd)
	opid := mustShieldCoinbase(t, ctx, jd, addr)
	txid := mustWaitOpTxID(t, ctx, jd, opid)
	raw := strings.TrimSpace(string(mustRunCLI(t, ctx, jd, "getrawtransaction", txid)))

	rpc := junocashd.New(jd.RPCURL, jd.RPCUser, jd.RPCPassword)
	bc, err := broadcast.New(rpc, broadcast.WithPollInterval(50*time.Millisecond))
	if err != nil {
		t.Fatalf("broadcast.New: %v", err)
	}

	gotTxID, err := bc.Submit(ctx, raw)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if gotTxID != txid {
		t.Fatalf("txid mismatch: got %q want %q", gotTxID, txid)
	}

	st, found, err := bc.Status(ctx, txid)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !found {
		t.Fatalf("expected tx to be found after submit")
	}
	if st.Confirmations != 0 {
		t.Fatalf("confirmations=%d want 0", st.Confirmations)
	}

	mustRunCLI(t, ctx, jd, "generate", "1")

	st, err = bc.WaitForConfirmations(ctx, txid, 1)
	if err != nil {
		t.Fatalf("WaitForConfirmations: %v", err)
	}
	if st.Confirmations < 1 {
		t.Fatalf("confirmations=%d want >=1", st.Confirmations)
	}
}

func mustCreateOrchardAddress(t *testing.T, ctx context.Context, jd *containers.Junocashd) string {
	t.Helper()

	var acc struct {
		Account int `json:"account"`
	}
	mustRunJSON(t, mustRunCLI(t, ctx, jd, "z_getnewaccount"), &acc)

	var addrResp struct {
		Address string `json:"address"`
	}
	mustRunJSON(t, mustRunCLI(t, ctx, jd, "z_getaddressforaccount", itoa(int64(acc.Account))), &addrResp)
	if strings.TrimSpace(addrResp.Address) == "" {
		t.Fatalf("missing address")
	}
	return strings.TrimSpace(addrResp.Address)
}

func mustShieldCoinbase(t *testing.T, ctx context.Context, jd *containers.Junocashd, toAddr string) string {
	t.Helper()

	var resp struct {
		OpID string `json:"opid"`
	}
	mustRunJSON(t, mustRunCLI(t, ctx, jd, "z_shieldcoinbase", "*", toAddr), &resp)
	if strings.TrimSpace(resp.OpID) == "" {
		t.Fatalf("missing opid")
	}
	return strings.TrimSpace(resp.OpID)
}

func mustWaitOpTxID(t *testing.T, ctx context.Context, jd *containers.Junocashd, opid string) string {
	t.Helper()

	deadline := time.Now().Add(2 * time.Minute)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}

	for time.Now().Before(deadline) {
		out := mustRunCLI(t, ctx, jd, "z_getoperationresult", `["`+opid+`"]`)
		var res []struct {
			Status string `json:"status"`
			Result *struct {
				TxID string `json:"txid"`
			} `json:"result,omitempty"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error,omitempty"`
		}
		if err := json.Unmarshal(out, &res); err == nil && len(res) > 0 {
			switch res[0].Status {
			case "success":
				if res[0].Result == nil {
					t.Fatalf("missing result in op success")
				}
				txid := strings.ToLower(strings.TrimSpace(res[0].Result.TxID))
				if len(txid) != 64 {
					t.Fatalf("invalid txid in op result: %q", txid)
				}
				return txid
			case "failed":
				msg := ""
				if res[0].Error != nil {
					msg = res[0].Error.Message
				}
				t.Fatalf("operation failed: %s (%s)", opid, msg)
			}
		}
		time.Sleep(200 * time.Millisecond)
	}

	t.Fatalf("operation did not succeed: %s", opid)
	return ""
}

func mustRunCLI(t *testing.T, ctx context.Context, jd *containers.Junocashd, args ...string) []byte {
	t.Helper()

	out, err := jd.ExecCLI(ctx, args...)
	if err != nil {
		t.Fatalf("junocash-cli %s: %v", filepath.Base(args[0]), err)
	}
	return out
}

func mustRunJSON(t *testing.T, b []byte, out any) {
	t.Helper()

	if err := json.Unmarshal(b, out); err != nil {
		t.Fatalf("unmarshal json: %v\n%s", err, string(b))
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		return "-" + itoa(-n)
	}
	var buf [32]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Abdullah1738/juno-broadcast/internal/testutil/containers"
)

func TestCLI_SubmitAndStatus_E2E(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
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
	rawFile := filepath.Join(t.TempDir(), "rawtx.hex")
	if err := os.WriteFile(rawFile, []byte(raw+"\n"), 0o600); err != nil {
		t.Fatalf("write raw tx: %v", err)
	}

	bin := filepath.Join("..", "..", "bin", "juno-broadcast")
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("missing binary: %v", err)
	}

	submitOut := mustRunBin(t, bin, "submit",
		"--rpc-url", jd.RPCURL,
		"--rpc-user", jd.RPCUser,
		"--rpc-pass", jd.RPCPassword,
		"--raw-tx-file", rawFile,
	)
	gotTxID := strings.TrimSpace(string(submitOut))
	if gotTxID != txid {
		t.Fatalf("txid mismatch: got %q want %q", gotTxID, txid)
	}

	mustRunCLI(t, ctx, jd, "generate", "1")

	var statusResp struct {
		Status string `json:"status"`
		Data   struct {
			TxID          string `json:"txid"`
			InMempool     bool   `json:"in_mempool"`
			Confirmations int64  `json:"confirmations"`
		} `json:"data"`
		Error any `json:"error"`
	}

	deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			out := mustRunBin(t, bin, "status",
				"--rpc-url", jd.RPCURL,
				"--rpc-user", jd.RPCUser,
				"--rpc-pass", jd.RPCPassword,
				"--txid", txid,
				"--json",
			)
			if err := json.Unmarshal(out, &statusResp); err != nil {
				t.Fatalf("invalid json: %v\n%s", err, string(out))
		}
		if statusResp.Status != "ok" {
			t.Fatalf("unexpected status response: %s", string(out))
		}
		if statusResp.Data.TxID != txid {
			t.Fatalf("txid mismatch")
		}
		if statusResp.Data.Confirmations >= 1 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

		t.Fatalf("timeout waiting for confirmations")
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
		t.Fatalf("junocash-cli %s: %v", args[0], err)
	}
	return out
}

func mustRunBin(t *testing.T, bin string, args ...string) []byte {
	t.Helper()

	cmd := exec.Command(bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run %s %s: %v\nstderr=%s\nstdout=%s", bin, strings.Join(args, " "), err, stderr.String(), stdout.String())
	}
	return stdout.Bytes()
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

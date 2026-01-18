package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Abdullah1738/juno-broadcast/internal/broadcast"
)

type fakeRunner struct {
	submit func(ctx context.Context, rawTxHex string) (string, error)
	status func(ctx context.Context, txid string) (broadcast.TxStatus, bool, error)
	wait   func(ctx context.Context, txid string, confirmations int64) (broadcast.TxStatus, error)
}

func (f fakeRunner) Submit(ctx context.Context, rawTxHex string) (string, error) {
	return f.submit(ctx, rawTxHex)
}

func (f fakeRunner) Status(ctx context.Context, txid string) (broadcast.TxStatus, bool, error) {
	return f.status(ctx, txid)
}

func (f fakeRunner) WaitForConfirmations(ctx context.Context, txid string, confirmations int64) (broadcast.TxStatus, error) {
	return f.wait(ctx, txid, confirmations)
}

func TestRun_Submit_RequiresRawTx(t *testing.T) {
	var out, errBuf bytes.Buffer

	code := RunWithIO([]string{"submit", "--rpc-url", "http://127.0.0.1:8232", "--json"}, func(string, string, string, time.Duration) (Runner, error) {
		t.Fatalf("factory should not be called")
		return nil, nil
	}, &out, &errBuf)

	if code == 0 {
		t.Fatalf("expected non-zero exit code")
	}
	if !strings.Contains(out.String(), `"version":"v1"`) {
		t.Fatalf("expected json version, got: %s", out.String())
	}
	if !strings.Contains(out.String(), `"status":"err"`) {
		t.Fatalf("expected json error, got: %s", out.String())
	}
}

func TestRun_Status_NotFound(t *testing.T) {
	var out, errBuf bytes.Buffer

	code := RunWithIO([]string{"status", "--rpc-url", "http://127.0.0.1:8232", "--txid", strings.Repeat("a", 64), "--json"}, func(string, string, string, time.Duration) (Runner, error) {
		return fakeRunner{
			status: func(ctx context.Context, txid string) (broadcast.TxStatus, bool, error) {
				return broadcast.TxStatus{}, false, nil
			},
		}, nil
	}, &out, &errBuf)

	if code == 0 {
		t.Fatalf("expected non-zero exit code")
	}
	if !strings.Contains(out.String(), `"version":"v1"`) {
		t.Fatalf("expected json version, got: %s", out.String())
	}
	if !strings.Contains(out.String(), `"code":"not_found"`) {
		t.Fatalf("expected not_found error, got: %s", out.String())
	}
}

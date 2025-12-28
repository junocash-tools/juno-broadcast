package broadcast

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Abdullah1738/juno-sdk-go/junocashd"
)

type fakeRPC struct {
	sendRawTransaction func(ctx context.Context, txHex string) (string, error)
	call              func(ctx context.Context, method string, params any, out any) error
}

func (f fakeRPC) Call(ctx context.Context, method string, params any, out any) error {
	if f.call == nil {
		return errors.New("fakeRPC: Call not set")
	}
	return f.call(ctx, method, params, out)
}

func (f fakeRPC) SendRawTransaction(ctx context.Context, txHex string) (string, error) {
	if f.sendRawTransaction == nil {
		return "", errors.New("fakeRPC: SendRawTransaction not set")
	}
	return f.sendRawTransaction(ctx, txHex)
}

func TestSubmit_ValidatesInputHex(t *testing.T) {
	c, err := New(fakeRPC{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := c.Submit(context.Background(), ""); err == nil {
		t.Fatalf("expected error for empty raw tx")
	}
	if _, err := c.Submit(context.Background(), "zz"); err == nil {
		t.Fatalf("expected error for non-hex raw tx")
	}
}

func TestSubmit_ValidatesTxID(t *testing.T) {
	c, err := New(fakeRPC{
		sendRawTransaction: func(ctx context.Context, txHex string) (string, error) {
			return "bad", nil
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := c.Submit(context.Background(), "00"); err == nil {
		t.Fatalf("expected error for invalid txid")
	}
}

func TestSubmit_NormalizesTxID(t *testing.T) {
	c, err := New(fakeRPC{
		sendRawTransaction: func(ctx context.Context, txHex string) (string, error) {
			return strings.ToUpper(strings.Repeat("a", 64)), nil
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	txid, err := c.Submit(context.Background(), "00")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if txid != strings.Repeat("a", 64) {
		t.Fatalf("txid=%q want %q", txid, strings.Repeat("a", 64))
	}
}

func TestStatus_FallbacksToMempool(t *testing.T) {
	txid := strings.Repeat("b", 64)

	var gotGetRawTx bool
	var gotMempool bool

	c, err := New(fakeRPC{
		call: func(ctx context.Context, method string, params any, out any) error {
			switch method {
			case "getrawtransaction":
				gotGetRawTx = true
				return &junocashd.RPCError{Code: -5, Message: "No such mempool or blockchain transaction"}
			case "getrawmempool":
				gotMempool = true
				dst := out.(*[]string)
				*dst = []string{strings.ToUpper(txid)}
				return nil
			default:
				return errors.New("unexpected method: " + method)
			}
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	st, found, err := c.Status(context.Background(), txid)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !found || !st.InMempool || st.Confirmations != 0 {
		t.Fatalf("unexpected status: %+v found=%v", st, found)
	}
	if !gotGetRawTx || !gotMempool {
		t.Fatalf("expected getrawtransaction and getrawmempool calls")
	}
}

func TestWaitForConfirmations_ZeroReturnsOnMempool(t *testing.T) {
	txid := strings.Repeat("c", 64)

	var calls int
	c, err := New(fakeRPC{
		call: func(ctx context.Context, method string, params any, out any) error {
			switch method {
			case "getrawtransaction":
				return &junocashd.RPCError{Code: -5, Message: "No such mempool or blockchain transaction"}
			case "getrawmempool":
				calls++
				dst := out.(*[]string)
				*dst = []string{txid}
				return nil
			default:
				return errors.New("unexpected method: " + method)
			}
		},
	}, WithPollInterval(1*time.Millisecond))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	st, err := c.WaitForConfirmations(ctx, txid, 0)
	if err != nil {
		t.Fatalf("WaitForConfirmations: %v", err)
	}
	if !st.InMempool {
		t.Fatalf("expected in mempool")
	}
	if calls == 0 {
		t.Fatalf("expected mempool check")
	}
}


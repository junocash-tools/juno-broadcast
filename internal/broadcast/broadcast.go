package broadcast

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Abdullah1738/juno-sdk-go/junocashd"
)

type TxStatus struct {
	TxID          string `json:"txid"`
	InMempool     bool   `json:"in_mempool"`
	Confirmations int64  `json:"confirmations"`
	BlockHash     string `json:"blockhash,omitempty"`
}

type RPC interface {
	Call(ctx context.Context, method string, params any, out any) error
	SendRawTransaction(ctx context.Context, txHex string) (string, error)
}

type Client struct {
	rpc          RPC
	pollInterval time.Duration
}

type Option func(*Client)

func WithPollInterval(d time.Duration) Option {
	return func(c *Client) {
		if d > 0 {
			c.pollInterval = d
		}
	}
}

func New(rpc RPC, opts ...Option) (*Client, error) {
	if rpc == nil {
		return nil, errors.New("broadcast: rpc is nil")
	}
	c := &Client{
		rpc:          rpc,
		pollInterval: 500 * time.Millisecond,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	return c, nil
}

func (c *Client) Submit(ctx context.Context, rawTxHex string) (string, error) {
	raw, err := normalizeHex(rawTxHex)
	if err != nil {
		return "", err
	}

	txid, err := c.rpc.SendRawTransaction(ctx, raw)
	if err != nil {
		return "", err
	}

	txid = strings.ToLower(strings.TrimSpace(txid))
	if _, err := hex.DecodeString(txid); err != nil || len(txid) != 64 {
		return "", errors.New("broadcast: node returned invalid txid")
	}
	return txid, nil
}

func (c *Client) Status(ctx context.Context, txid string) (TxStatus, bool, error) {
	txid = strings.ToLower(strings.TrimSpace(txid))
	if _, err := hex.DecodeString(txid); err != nil || len(txid) != 64 {
		return TxStatus{}, false, errors.New("broadcast: txid must be 32-byte hex")
	}

	// Prefer a direct lookup (works for mempool + chain with -txindex=1).
	var verbose struct {
		TxID          string `json:"txid"`
		BlockHash     string `json:"blockhash"`
		Confirmations int64  `json:"confirmations"`
	}
	if err := c.rpc.Call(ctx, "getrawtransaction", []any{txid, 1}, &verbose); err == nil {
		return TxStatus{
			TxID:          txid,
			InMempool:     verbose.Confirmations == 0 && verbose.BlockHash == "",
			Confirmations: verbose.Confirmations,
			BlockHash:     strings.TrimSpace(verbose.BlockHash),
		}, true, nil
	} else if !isNotFoundErr(err) {
		return TxStatus{}, false, err
	}

	// Fallback: check mempool membership.
	var mempool []string
	if err := c.rpc.Call(ctx, "getrawmempool", []any{false}, &mempool); err != nil {
		return TxStatus{}, false, fmt.Errorf("broadcast: getrawmempool: %w", err)
	}
	for _, id := range mempool {
		if strings.ToLower(strings.TrimSpace(id)) == txid {
			return TxStatus{
				TxID:          txid,
				InMempool:     true,
				Confirmations: 0,
			}, true, nil
		}
	}

	return TxStatus{}, false, nil
}

func (c *Client) WaitForConfirmations(ctx context.Context, txid string, confirmations int64) (TxStatus, error) {
	if confirmations < 0 {
		return TxStatus{}, errors.New("broadcast: confirmations must be >= 0")
	}

	ticker := time.NewTicker(c.pollInterval)
	defer ticker.Stop()

	for {
		st, found, err := c.Status(ctx, txid)
		if err != nil {
			return TxStatus{}, err
		}
		if found && (confirmations == 0 || st.Confirmations >= confirmations) {
			return st, nil
		}

		select {
		case <-ctx.Done():
			return TxStatus{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

func normalizeHex(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", errors.New("broadcast: raw tx hex is required")
	}
	if _, err := hex.DecodeString(s); err != nil {
		return "", errors.New("broadcast: raw tx hex must be hex")
	}
	return s, nil
}

func isNotFoundErr(err error) bool {
	var rpcErr *junocashd.RPCError
	if !errors.As(err, &rpcErr) {
		return false
	}
	msg := strings.ToLower(rpcErr.Message)
	return strings.Contains(msg, "no such mempool") ||
		strings.Contains(msg, "no such") ||
		strings.Contains(msg, "not found")
}

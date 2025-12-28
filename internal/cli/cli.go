package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Abdullah1738/juno-broadcast/internal/broadcast"
	"github.com/Abdullah1738/juno-sdk-go/junocashd"
)

type Runner interface {
	Submit(ctx context.Context, rawTxHex string) (string, error)
	Status(ctx context.Context, txid string) (broadcast.TxStatus, bool, error)
	WaitForConfirmations(ctx context.Context, txid string, confirmations int64) (broadcast.TxStatus, error)
}

type Factory func(rpcURL, rpcUser, rpcPass string, pollInterval time.Duration) (Runner, error)

func Run(args []string) int {
	return RunWithIO(args, defaultFactory, os.Stdout, os.Stderr)
}

func RunWithIO(args []string, factory Factory, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		writeUsage(stdout)
		return 2
	}

	switch args[0] {
	case "-h", "--help", "help":
		writeUsage(stdout)
		return 0
	case "submit":
		return runSubmit(args[1:], factory, stdout, stderr)
	case "status":
		return runStatus(args[1:], factory, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n\n", args[0])
		writeUsage(stderr)
		return 2
	}
}

func writeUsage(w io.Writer) {
	fmt.Fprintln(w, "juno-broadcast")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Submit signed raw transactions to junocashd and report status.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  juno-broadcast submit --rpc-url <url> --rpc-user <user> --rpc-pass <pass> --raw-tx-hex <hex> [--confirmations <n>] [--poll <duration>] [--json]")
	fmt.Fprintln(w, "  juno-broadcast status --rpc-url <url> --rpc-user <user> --rpc-pass <pass> --txid <txid> [--json]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Env:")
	fmt.Fprintln(w, "  JUNO_RPC_URL, JUNO_RPC_USER, JUNO_RPC_PASS")
}

func runSubmit(args []string, factory Factory, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("submit", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var rpcURL string
	var rpcUser string
	var rpcPass string
	var rawTxHex string
	var rawTxFile string
	var confirmations int64
	var pollStr string
	var jsonOut bool

	fs.StringVar(&rpcURL, "rpc-url", "", "junocashd RPC URL")
	fs.StringVar(&rpcUser, "rpc-user", "", "junocashd RPC username")
	fs.StringVar(&rpcPass, "rpc-pass", "", "junocashd RPC password")
	fs.StringVar(&rawTxHex, "raw-tx-hex", "", "signed raw tx hex")
	fs.StringVar(&rawTxFile, "raw-tx-file", "", "path to file containing signed raw tx hex")
	fs.Int64Var(&confirmations, "confirmations", 0, "wait for N confirmations (0 = don't wait)")
	fs.StringVar(&pollStr, "poll", "500ms", "poll interval (e.g. 500ms, 2s)")
	fs.BoolVar(&jsonOut, "json", false, "JSON output")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 2
	}

	rpcURL, rpcUser, rpcPass, err := rpcConfigFromFlags(rpcURL, rpcUser, rpcPass)
	if err != nil {
		return writeErr(stdout, stderr, jsonOut, "invalid_request", err.Error())
	}

	raw, err := loadHexInput(rawTxHex, rawTxFile, "raw-tx-hex", "raw-tx-file")
	if err != nil {
		return writeErr(stdout, stderr, jsonOut, "invalid_request", err.Error())
	}

	poll, err := time.ParseDuration(pollStr)
	if err != nil {
		return writeErr(stdout, stderr, jsonOut, "invalid_request", "poll must be a duration")
	}

	r, err := factory(rpcURL, rpcUser, rpcPass, poll)
	if err != nil {
		return writeErr(stdout, stderr, jsonOut, "internal", err.Error())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	txid, err := r.Submit(ctx, raw)
	if err != nil {
		return writeErr(stdout, stderr, jsonOut, "node_rpc_error", err.Error())
	}

	if confirmations > 0 {
		st, err := r.WaitForConfirmations(ctx, txid, confirmations)
		if err != nil {
			return writeErr(stdout, stderr, jsonOut, "node_rpc_error", err.Error())
		}
		return writeOK(stdout, jsonOut, map[string]any{
			"txid":          txid,
			"in_mempool":     st.InMempool,
			"confirmations":  st.Confirmations,
			"blockhash":      st.BlockHash,
			"required_confs": confirmations,
		})
	}

	if jsonOut {
		return writeOK(stdout, jsonOut, map[string]any{"txid": txid})
	}
	fmt.Fprintln(stdout, txid)
	return 0
}

func runStatus(args []string, factory Factory, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var rpcURL string
	var rpcUser string
	var rpcPass string
	var txid string
	var jsonOut bool
	var pollStr string

	fs.StringVar(&rpcURL, "rpc-url", "", "junocashd RPC URL")
	fs.StringVar(&rpcUser, "rpc-user", "", "junocashd RPC username")
	fs.StringVar(&rpcPass, "rpc-pass", "", "junocashd RPC password")
	fs.StringVar(&txid, "txid", "", "transaction id")
	fs.StringVar(&pollStr, "poll", "500ms", "poll interval (unused)")
	fs.BoolVar(&jsonOut, "json", false, "JSON output")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 2
	}

	rpcURL, rpcUser, rpcPass, err := rpcConfigFromFlags(rpcURL, rpcUser, rpcPass)
	if err != nil {
		return writeErr(stdout, stderr, jsonOut, "invalid_request", err.Error())
	}

	txid = strings.TrimSpace(txid)
	if txid == "" {
		return writeErr(stdout, stderr, jsonOut, "invalid_request", "txid is required")
	}

	poll, err := time.ParseDuration(pollStr)
	if err != nil {
		return writeErr(stdout, stderr, jsonOut, "invalid_request", "poll must be a duration")
	}

	r, err := factory(rpcURL, rpcUser, rpcPass, poll)
	if err != nil {
		return writeErr(stdout, stderr, jsonOut, "internal", err.Error())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st, found, err := r.Status(ctx, txid)
	if err != nil {
		return writeErr(stdout, stderr, jsonOut, "node_rpc_error", err.Error())
	}
	if !found {
		return writeErr(stdout, stderr, jsonOut, "not_found", "unknown txid")
	}

	return writeOK(stdout, jsonOut, st)
}

func defaultFactory(rpcURL, rpcUser, rpcPass string, pollInterval time.Duration) (Runner, error) {
	rpc := junocashd.New(rpcURL, rpcUser, rpcPass)
	return broadcast.New(rpc, broadcast.WithPollInterval(pollInterval))
}

func rpcConfigFromFlags(url, user, pass string) (string, string, string, error) {
	if strings.TrimSpace(url) == "" {
		url = os.Getenv("JUNO_RPC_URL")
	}
	if strings.TrimSpace(user) == "" {
		user = os.Getenv("JUNO_RPC_USER")
	}
	if strings.TrimSpace(pass) == "" {
		pass = os.Getenv("JUNO_RPC_PASS")
	}

	url = strings.TrimSpace(url)
	if url == "" {
		return "", "", "", errors.New("rpc-url is required (or set JUNO_RPC_URL)")
	}

	return url, user, pass, nil
}

func loadHexInput(hexValue, filePath, hexFlagName, fileFlagName string) (string, error) {
	var sources int
	if strings.TrimSpace(hexValue) != "" {
		sources++
	}
	if strings.TrimSpace(filePath) != "" {
		sources++
	}
	if sources == 0 {
		return "", fmt.Errorf("%s is required (or use --%s)", hexFlagName, fileFlagName)
	}
	if sources > 1 {
		return "", fmt.Errorf("input source conflict (use only one of --%s, --%s)", hexFlagName, fileFlagName)
	}
	if strings.TrimSpace(hexValue) != "" {
		return strings.TrimSpace(hexValue), nil
	}

	path := strings.TrimSpace(filePath)
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	return strings.TrimSpace(string(b)), nil
}

func writeOK(w io.Writer, jsonOut bool, payload any) int {
	if !jsonOut {
		b, _ := json.Marshal(payload)
		fmt.Fprintln(w, string(b))
		return 0
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": "ok",
		"data":   payload,
	})
	return 0
}

func writeErr(stdout, stderr io.Writer, jsonOut bool, code, msg string) int {
	if jsonOut {
		_ = json.NewEncoder(stdout).Encode(map[string]any{
			"status": "err",
			"error": map[string]any{
				"code":    code,
				"message": msg,
			},
		})
		return 1
	}
	if msg == "" {
		msg = code
	}
	fmt.Fprintln(stderr, msg)
	return 1
}

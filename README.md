# juno-broadcast

Submit signed raw transactions to `junocashd` and track status.

Used for withdrawals and sweeps/rebalances; no key material required.

## CLI

Prereqs:

- a running `junocashd` with RPC enabled

Commands:

- Submit: `juno-broadcast submit --rpc-url <url> --rpc-user <user> --rpc-pass <pass> --raw-tx-hex <hex>`
- Status: `juno-broadcast status --rpc-url <url> --rpc-user <user> --rpc-pass <pass> --txid <txid>`
- Serve HTTP API: `juno-broadcast serve --rpc-url <url> --rpc-user <user> --rpc-pass <pass> --listen 127.0.0.1:8080`

Set `JUNO_RPC_URL`, `JUNO_RPC_USER`, and `JUNO_RPC_PASS` to avoid passing flags.

## HTTP API

- `GET /healthz`
- `POST /v1/tx/submit` (`{"raw_tx_hex":"...","wait_confirmations":1}`)
- `GET /v1/tx/{txid}`

# juno-broadcast

Submit signed raw transactions to `junocashd` and track status.

Used for withdrawals and sweeps/rebalances; no key material required.

Status: work in progress.

## CLI

Prereqs:

- a running `junocashd` with RPC enabled

Commands:

- Submit: `juno-broadcast submit --rpc-url <url> --rpc-user <user> --rpc-pass <pass> --raw-tx-hex <hex>`
- Status: `juno-broadcast status --rpc-url <url> --rpc-user <user> --rpc-pass <pass> --txid <txid>`

Set `JUNO_RPC_URL`, `JUNO_RPC_USER`, and `JUNO_RPC_PASS` to avoid passing flags.

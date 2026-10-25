# Attestation tunnel cutover tools

The AWS gateway calls a loopback attestation queue, which forwards through an SSH reverse tunnel to the Mac helper. Use a stable Tailscale destination for that existing SSH tunnel, retaining its restricted key, localhost listener, and pinned host key. Tailscale SSH remains disabled. Do not send account credentials, SDP or audio to the helper.

## Preflight and isolation

Back up the active production plist and its hash before making a candidate. Verify the pinned AWS host key and restricted-key authentication over the private route without opening a forward. Use a separate candidate LaunchAgent, restricted temporary key, loopback reverse port and queue port. The candidate key must never become the production key. Mirror only the production queue's non-secret configuration switches.

Require authenticated candidate health, one successful proof whose contents are discarded in memory, unauthorized/payload rejection, supervised reconnect, and a real candidate rollback before changing production. Check production health throughout. Do not stop the helper, production queue, gateway or database.

## Rollback guard

Run `rollback_guard.py guard STATE_DIRECTORY` as an independent temporary user LaunchAgent **before** changing the selected tunnel. Its directory must be below the helper runtime at `~/Library/Application Support/InfernoAttestationAWS`, private to the operator. Put the exact original bytes in `backup.plist`, retaining their mode. The configuration is:

```json
{
  "label": "com.opencomputer.inferno-attestation-aws-tunnel-candidate",
  "backup_sha256": "SHA256_OF_BACKUP_BYTES",
  "candidate_sha256": "SHA256_OF_NEW_ACTIVE_PLIST_BYTES",
  "delay_seconds": 180
}
```

Use a fresh state directory per attempt. `confirm STATE_DIRECTORY` atomically cancels the guard after independent gateway-path checks pass. If the timeout claimed rollback first, confirmation fails. `restore STATE_DIRECTORY` allows an explicit later restore, provided the target still matches the recorded candidate. The guard refuses unrelated labels, changed backups or concurrent edits. It only unloads and reloads the selected tunnel.

Inspect `result.json`: a failed OS command is a failed rollback, never a successful restoration. A bootstrap failure can leave the original file restored but the job unloaded. Inspect that exact label and its child SSH process/listener, resolve any stale child, then bootstrap the backed-up job and check the gateway route. Never kill an unverified PID. Test this procedure on the isolated candidate before production.

Keep the existing narrow public Air SSH rule until the private route and production proof pass. Record its complete definition. A later public-route rollback after removal must first restore that rule for the Air's then-current public IP.

## Private network firewall

Use Tailscale with DNS acceptance, route acceptance, Tailscale SSH and automatic netfilter management disabled; do not advertise routes or use an exit node. Disable the server node's key expiry in the admin console. Audit actual container bindings before applying host INPUT rules: Docker-published non-loopback ports need a separately reviewed FORWARD/DOCKER-USER policy. Host-network services and loopback-only container bindings are covered by the supplied configuration.

The service allows IPv4 SSH from the Air and the existing operator Mac only. The operator entry intentionally retains administrative and rollback access after the obsolete public rule is removed. Other new tailnet ingress is dropped; established replies are preserved. Review these stable private addresses for each deployment. To change them, run the script with `--remove OLD_AIR_IPV4 OLD_OPERATOR_IPV4`, edit `ExecStart`, run `systemctl daemon-reload`, then apply the script directly with the new addresses; never `systemctl restart` the firewall unit for this: `Requires=` restarts tailscaled with it, and with the old chain still present the script refuses, the unit fails and tailscaled stays stopped (no attestation tunnel, no tailnet admin access).

Install `tailnet-firewall.sh` as `/usr/local/sbin/inferno-attestation-tailnet-firewall` and the provided service/drop-in in their matching systemd locations. Enable/start the firewall unit explicitly, then verify idempotency and boot ordering. On an already running production host, reapply the script directly and keep the dependency unit running. Do not restart tailscaled or customer services just to apply the drop-in. The script refuses an existing owned chain whose rules differ; it never silently accumulates source addresses. `--remove AIR_IPV4 OPERATOR_IPV4` removes only its exact validated rules, jumps and chains. Restore a verified public tunnel before stopping the dependency or removing Tailscale protection.

## Verification

```sh
python3 -m unittest discover -s deploy/attestation/tests -v
sh -n deploy/attestation/tailnet-firewall.sh
git diff --check
```

On an authorized Linux test host, exercise firewall application, idempotency, unexpected-chain rejection and exact teardown inside an isolated network namespace. Compare all namespace rules before/after, including unrelated sentinel rules. Then verify actual private reachability, blocked non-SSH ingress, unchanged production processes and unchanged Docker FORWARD rules. Remove candidate jobs, keys and listeners after migration; retain the exact rollback backup and receipts without tokens or proofs.

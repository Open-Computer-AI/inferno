# Attestation failover staging candidate

This directory contains the two-helper attestation race and its isolated test harness. It runs on loopback and is enabled by supplying a local `QUEUE_RACE_CONFIG`. The existing gateway release script uses its own hash-pinned queue in the parent directory.

## Source correlation

| Queue baseline | SHA-256 |
| --- | --- |
| Queue in the parent directory, pinned by `gateway_queue_v2.sh` | `9205dfc2f0cda5a9c63db482816c4ffe7ed96bbbd2efa6cfbdb2302eec9f697d` |
| Verified AWS queue snapshot used as the staging base | `03452f3c2c7e415f026da01586e93ec7bd97a86c473dc84c7455d4fe0aa64803` |

The AWS snapshot adds idle connection warming, health refreshes, and warm-pool counters to the parent implementation. The initial staging commit captures that snapshot byte for byte. The following commit adds the opt-in race. It keeps one gateway request and returns the first valid proof from up to two concurrent helper attempts. An explicit failure dispatches the backup immediately, including when the head start is set to 200 ms.

## Validation

The isolated gateway benchmark used a hash-matched Linux ARM64 gateway, a fresh test database, and a synthetic TLS/SDP upstream on its own internal Docker network. With the simulated Air tunnel open but silent, the immediate race had lower p50 and p95 at each concurrency.

| Concurrent Live starts | 0 ms race p50 / p95 | 200 ms head start p50 / p95 | Starts per policy |
| ---: | ---: | ---: | ---: |
| 1 | 113 / 121 ms | 317 / 326 ms | 12 |
| 4 | 118 / 165 ms | 350 / 394 ms | 48 |
| 16 | 162 / 272 ms | 369 / 682 ms | 192 |

Across silent stalls and hard disconnects, the gateway completed 1,008 measured starts with one upstream create per successful start and zero failures. A separate 168-start replay ended with zero gateway concurrency leases. A native macOS helper generated 1,680 distinct proofs over loopback with zero failures. Its p95 favored the immediate race during stalls at all three loads, and its p99 was mixed at 4 and 16.

The 0 ms race is the candidate for silent stalls. Hard disconnects produced similar timings. These measurements cover local request latency through the queue and gateway fixture. Independent physical helper networks and actual upstream Live media remain future staging checks.

## Reproduce

Run these commands from the staging directory.

```sh
python3 -B -m unittest -v test_attest_race
python3 -B stage_attestation.py --waves 2 --faults stall reset \
  --output /tmp/attestation-synthetic-stage.json
python3 -B stage_attestation.py --waves 20 --faults stall reset \
  --native-helper /path/to/qualified-macos-attestation-helper \
  --output /tmp/attestation-native-stage.json
python3 -B stage_gateway.py --waves 12 \
  --gateway /path/to/verified-linux-arm64-gateway \
  --output /tmp/attestation-gateway-stage.json
```

The gateway harness verifies the binary hash and uses `postgres:17`, `redis:7-alpine`, and `python:3.13-slim`. Its temporary containers and network are labeled and removed on exit. The native harness starts a temporary loopback helper and stops it on exit. Each command uses generated staging credentials.

# Runbook — Phase 0: prove the pipe

**Goal:** stand up one real WireGuard exit gateway and confirm full-tunnel
traffic works end to end, *before* any application code. This validates the
network model (WireGuard + NAT + DNS + MTU + full-tunnel routing) that every
later phase depends on.

**Time:** ~30 minutes. **You provide:** one fresh Debian 12 / Ubuntu 22.04+ VPS
and a macOS or Linux laptop.

---

## 0. Pick a host (provider-agnostic)

Any VPS works, but the exit node moves a lot of traffic, so favour
**VPN-tolerant hosts with generous/unmetered egress**:

| Host | Why | Watch out |
|------|-----|-----------|
| **Hetzner** | Cheap, ~20 TB traffic included | VPN OK; keep abuse policy clean |
| **Vultr** | Many regions, high bandwidth | Per-plan bandwidth caps |
| **DigitalOcean** | Simple, many regions | 1–5 TB caps, then per-GB |
| AWS/GCP/Azure | Familiar | **Per-GB egress will dominate cost** — avoid for exit nodes |

Create the smallest 2 vCPU / 2–4 GB instance. Note its **public IPv4**.

## 1. Open the firewall

In the provider's firewall / security group, allow inbound **UDP 51820**
(and keep SSH/TCP 22). This is the single most common reason Phase 0 "hangs":
the handshake never arrives.

## 2. Copy the kit and bootstrap

From your laptop, in this repo:

```bash
scp -r infra/gateway root@YOUR_VPS_IP:~/aevora-gateway
ssh root@YOUR_VPS_IP
cd aevora-gateway
cp config.env.example config.env
# Defaults are sane. Optionally set PUBLIC_ENDPOINT to a hostname if you have DNS.
./bootstrap.sh
```

`bootstrap.sh` installs WireGuard, enables forwarding, generates the gateway
keypair, writes the interface + NAT rules, and brings the tunnel up. It prints
the **gateway public key** and **endpoint** at the end. Re-running it is safe —
it preserves existing peers.

## 3. Add your laptop as a peer

```bash
./add-peer.sh my-laptop
```

This assigns the next tunnel IP, adds the peer live and on-disk, and writes
`clients/my-laptop.conf`. On a machine with `qrencode`, it also prints a QR code.

> **Two modes.** The command above (no key argument) generates the client key on
> the gateway for speed — a Phase-0 convenience. The production-correct way,
> which the control plane will use, keeps the private key on the client:
>
> ```bash
> # On the LAPTOP:
> wg genkey | tee laptop.key | wg pubkey    # copy the printed public key
> # On the GATEWAY:
> ./add-peer.sh my-laptop <that-public-key>
> # Then paste your laptop.key into the [Interface] PrivateKey line of the .conf.
> ```

## 4. Connect the client

**macOS** — install the official WireGuard app (App Store or `brew install --cask wireguard`),
then *Import tunnel(s) from file…* → `my-laptop.conf`, and toggle it on. Or scan
the QR from the WireGuard mobile app for a phone test.

**Linux** — copy the conf and bring it up:

```bash
sudo cp my-laptop.conf /etc/wireguard/aevora.conf
sudo wg-quick up aevora
# stop with: sudo wg-quick down aevora
```

The config uses `AllowedIPs = 0.0.0.0/0, ::/0` — **full tunnel**, all traffic
routed through the gateway.

## 5. Validate

**On the gateway:**

```bash
./validate.sh
```

Expect all checks green: forwarding on (v4+v6), interface up, listening on
51820, NAT masquerade present, MSS clamp present. It also shows the peer's
latest handshake — a recent timestamp means the client connected.

**On the laptop (the real proof):**

```bash
curl -s https://api.ipify.org ; echo          # should print the GATEWAY's IP
curl -s https://1.1.1.1/cdn-cgi/trace | grep ip=   # cross-check the exit IP
dig +short whoami.akamai.net @9.9.9.9          # DNS resolves through the tunnel
```

Then just browse for a minute and run a quick speed test.

---

## Pass / fail criteria

Phase 0 **passes** when all of these hold:

- [ ] `validate.sh` reports 0 failures.
- [ ] Client's public IP (`api.ipify.org`) equals the **gateway's** public IP.
- [ ] `latest handshake` for the peer updates within the last ~2 minutes.
- [ ] Web browsing and DNS work normally through the tunnel.
- [ ] A speed test shows usable throughput (sanity, not a benchmark).
- [ ] No DNS leak: queries resolve via the tunnel DNS, not your local ISP.

If any fail, see Troubleshooting.

## Troubleshooting

| Symptom | Likely cause | Fix |
|--------|--------------|-----|
| No handshake ever | UDP 51820 blocked | Open it in the provider firewall (step 1) |
| Handshake OK, no internet | Forwarding/NAT off | Re-run `bootstrap.sh`; check `validate.sh` |
| Some sites hang / slow TLS | MTU / MSS | Confirm MSS clamp in `validate.sh`; try `MTU=1380` in the client |
| Exit IP is still yours | Not full-tunnel | Confirm `AllowedIPs = 0.0.0.0/0, ::/0` in the client conf |
| DNS leaks to ISP | Client not forcing tunnel DNS | Ensure `DNS =` line present; enable the app's "block untunneled" / kill-switch |

## What Phase 0 does *not* include (by design)

Automation, the control plane, server selection, the desktop app — all come in
Phases 1–2. Phase 0 is purely: *does a single hand-built gateway carry real
traffic correctly?* Once it does, we automate it.

# Production readiness plans (queued)

Software scaffolding complete. **HIL harnesses** for Minewing device-agent +
OTA RAUC/power-loss are landed; **silicon sign-off still requires the board
image / physical unit** (lab host is x86 surrogate only).

---

## Device-agent / OTA HIL

| Item | Status |
|---|---|
| `zyvor-device-agent` HIL runner | **done** — `scripts/hil/run-minewing-hil.sh` |
| Lab surrogate evidence | **recorded** — not claimable |
| Minewing physical sign-off | **blocked** — need aarch64 board + profile |
| `zyvor-ota` RAUC/power-loss runner | **done** — `scripts/hil/run-rauc-powerloss-hil.sh` |
| QEMU+RAUC image | **blocked** — set `QUALIFY_QEMU_IMAGE` |
| Checklist auto-sign | fail-closed until claimable |

---

## Suggested next

Provide Minewing QEMU image path + SSH to guest (or physical board), re-run
HIL with logs attached, then `DA_HIL_SIGN=1` / `OTA_HIL_SIGN=1`.

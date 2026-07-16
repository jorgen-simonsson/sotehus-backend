# SOLIS MQTT Payload

Describes the JSON payload published by `solis2mqtt` with the current
`src/config/config.json`. This config defines a single device (`SOLIS`) on a
single Modbus RTU link and one register table (`Solis_Registers`).

Register meanings and sign conventions below are taken from
`doc/RS485_MODBUS(ESINV-33000ID) Hybrid Inverter.pdf`, cross-referenced
against the register addresses in `config.json`.

## MQTT topic

| Setting | Value |
|---|---|
| Topic | `solis/modbus` |
| QoS | 1 |
| Retained | yes |
| Publish interval | every `pollingInterval` (currently 1000 ms), once per successful poll round |
| Payload | one flat JSON object, merging every register cluster read that round |

A cluster or device read failure is logged and that round's payload simply
omits the corresponding properties — it does not block publishing of the
other clusters, and does not stop the poller.

## Payload shape

The payload is a single flat JSON object; property names such as
`voltage.L1` are literal dot-containing JSON keys, **not** nested objects:

```json
{
  "voltage.L1": 231.20,
  "current.L1": 2.34,
  "voltage.L2": 230.80,
  "current.L2": 1.87,
  "voltage.L3": 229.90,
  "current.L3": 2.01,
  "power.L1": 512.00,
  "power.L2": 398.00,
  "power.L3": -120.00,
  "power.total": 790.00,
  "meter.type": 258.00,
  "battery.SOC": 85.00,
  "battery.power": -430.00,
  "solar.power": 3120.00
}
```

All values are formatted to a fixed number of decimals (2, the daemon
default — no register in this config overrides `outputDecimals`), even
where the underlying register's real precision is coarser (e.g. voltage is
only accurate to 0.1 V but is still printed with 2 decimals).

## Properties

Source device for every register below is the external meter/CT wired to
the Solis inverter's grid-side meter port (registers 33251–33286), and the
inverter's own battery/port telemetry (registers 33139, 34605, 34621).

| JSON property | Register(s) | Modbus FC | Type | Unit | Sign meaning |
|---|---|---|---|---|---|
| `voltage.L1` | 33251 | 4 (input) | uint16, ×0.1 | V | Always positive (magnitude only). |
| `current.L1` | 33252 | 4 (input) | uint16, ×0.01 | A | Always positive (magnitude only). |
| `voltage.L2` | 33253 | 4 (input) | uint16, ×0.1 | V | Always positive (magnitude only). |
| `current.L2` | 33254 | 4 (input) | uint16, ×0.01 | A | Always positive (magnitude only). |
| `voltage.L3` | 33255 | 4 (input) | uint16, ×0.1 | V | Always positive (magnitude only). |
| `current.L3` | 33256 | 4 (input) | uint16, ×0.01 | A | Always positive (magnitude only). |
| `power.L1` | 33257–33258 | 4 (input) | int32, ×1 | W | **Signed.** Positive = phase A power flowing *toward* the public grid (export). Negative = power being *drawn from* the grid (import) on that phase. |
| `power.L2` | 33259–33260 | 4 (input) | int32, ×1 | W | Same convention as `power.L1`, phase B. |
| `power.L3` | 33261–33262 | 4 (input) | int32, ×1 | W | Same convention as `power.L1`, phase C. |
| `power.total` | 33263–33264 | 4 (input) | int32, ×1 | W | Sum of all phases. Positive = net export to the public grid; negative = net import from the grid. |
| `meter.type` | 43140 | 3 (holding) | int16, ×1 | — | Not a magnitude — a bit-packed enum. High byte = meter installation location (`0x01`=grid side, `0x02`=reserved, `0x03`=grid side + parallel inverter output, dual meters); low byte = meter model (`0x01`=Acrel single-phase, `0x02`=Acrel three-phase, `0x04`=Eastron single-phase, `0x05`=Eastron three-phase, `0x06`=no meter, `0x07`=Chint split-phase, `0x08`=Chint dual-channel). Decode by splitting the raw register value into high/low bytes; the sign bit is never set for any defined combination, so the `int16` vs `uint16` declaration in config has no practical effect here. |
| `battery.SOC` | 33139 | 4 (input) | uint16, ×1 | % | Always positive, 0–100. |
| `battery.power` | 34605–34606 | 4 (input) | int32, ×1 | W | **Signed.** Positive = battery *charging*. Negative = battery *discharging*. Raw value `0x80000000` (i.e. −2147483648) is a sentinel meaning "invalid/no battery" rather than an actual power reading — treat that specific value as absent data, not as a real ~−2.1 GW reading. |
| `solar.power` | 34621–34622 | 4 (input) | int32, ×1 | W | Vendor register name is "Grid-tied Inverter Output Total Active Power" (register 34621), not a dedicated PV/solar register (that would be 34603–34604, "PV Power", not read by this config). Unlike the other signed registers here, the sign is **not** directional: per the vendor doc, only positive values are valid readings; negative values (and the sentinel `0x80000000`) indicate no grid-tied inverter is connected / the reading is invalid, not power flowing in reverse. |

## Notes on precision vs. displayed decimals

- `voltage.*` registers carry 0.1 V of real precision (raw register × 0.1)
  but are printed with 2 decimals (e.g. `231.20`) since `outputDecimals` is
  not set for them in `config.json`. The trailing `0` is not extra
  precision.
- `current.*` registers carry 0.01 A of real precision, which matches the
  2-decimal default exactly.
- All `power.*`, `battery.power`, `solar.power`, `battery.SOC` and
  `meter.type` registers have a scale factor of 1 (whole units) but are
  still printed with 2 decimals (e.g. `790.00`) for the same reason.

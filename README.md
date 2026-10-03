# StressLab

**How much can one small machine *really* take — and how much more can you squeeze out of it?**

One Go API, one Postgres, one Azure VM. Find the limit, fix what broke, measure again — one
change at a time. Scale out only when the numbers say one machine is out of road.

**[Spec](docs/SPEC.md) · [Methodology](docs/METHODOLOGY.md) · [Architecture](docs/ARCHITECTURE.md) · [Roadmap](docs/ROADMAP.md)**

> 🚧 **Status: building the lab.** No runs yet — every number here will come from a committed
> run in `runs/`.

## Results

| Step | Change | Max users | Req/s | p95 | What broke next | $ / hour |
|---|---|---|---|---|---|---|
| 0 | Baseline | — | — | — | — | — |

*Generated from `runs/` by CI — never typed by hand.*

## The ladder

```mermaid
flowchart TB
  subgraph ACT1["Act 1 · squeeze one machine dry"]
    direction LR
    S0["0 Baseline"] --> S1["1 Queries"] --> S2["2 Pooling"] --> S3["3 Profiling"] --> S4["4 Cache"]
  end
  subgraph ACT2["Act 2 · when one machine is not enough"]
    direction LR
    S5["5 Managed DB"] --> S6["6 Replica"] --> S7["7 Scale out"] --> S8["8 Kafka"] --> S9["9 Fan-out"] --> S10["10 Overload"]
  end
  ACT1 ==>|"out of road"| ACT2

  classDef act1 fill:#dbeafe,stroke:#2563eb,color:#1e3a8a
  classDef act2 fill:#fef3c7,stroke:#d97706,color:#78350f
  class S0,S1,S2,S3,S4 act1
  class S5,S6,S7,S8,S9,S10 act2
```

## The lab

```mermaid
flowchart LR
  subgraph BRAIN["VM: brain"]
    K6["k6<br/>simulated users"]
    subgraph OBS["observability"]
      direction TB
      PROM[("Prometheus<br/>metrics")]
      LOKI[("Loki<br/>logs")]
      TEMPO[("Tempo<br/>traces")]
      PYRO[("Pyroscope<br/>profiles")]
    end
    GRAF["Grafana"]
    OBS --> GRAF
  end
  subgraph BOX["VM: machine under test"]
    API["Go API"]
    PG[("Postgres")]
    API --> PG
  end
  K6 -->|"HTTP, private network"| API
  BOX -.->|"telemetry"| OBS
```

**Stack:** Go · Postgres · Redis · Kafka (Event Hubs) · k6 · Prometheus · Loki · Tempo ·
Pyroscope · Grafana · Docker · Terraform · Ansible · GitHub Actions · Azure

---

Inspired by Arjay's [I Tested 8 Programming Languages on a $12 Server](https://www.youtube.com/watch?v=sQXFhh_PiG4) —
StressLab holds the language still and asks what else there is to squeeze.

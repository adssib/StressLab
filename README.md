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

## How it's measured

- **One change per step.** Same app, data, load and machine — only the thing being tested differs.
- **The bar:** p95 < 500 ms, p99 < 1 s, errors < 1%. A level counts only if **three 5-minute runs
  all pass**.
- **Past the limit too:** load at up to 2.5× the limit shows whether the system bends or
  collapses — with and without load shedding.
- **Honest runs:** runs where the load generator or the hardware was the problem are marked
  invalid, not counted. Steps that didn't help still get a row.

## The ladder

```mermaid
flowchart TB
  subgraph ACT1["Act 1 · squeeze one machine dry"]
    direction LR
    S0["0 Baseline"] --> S1["1 Queries"] --> S2["2 Pooling"] --> S3["3 Profiling"] --> S4["4 Cache"]
  end
  subgraph ACT2["Act 2 · when one machine is not enough"]
    direction LR
    S5["5 Managed DB"] --> S6["6 Read replica"] --> S7["7 Scale out"] --> S8["8 Async writes"] --> S9["9 Fan-out"] --> S10["10 Overload"]
  end
  ACT1 ==>|"out of road"| ACT2

  classDef act1 fill:#dbeafe,stroke:#2563eb,color:#1e3a8a
  classDef act2 fill:#fef3c7,stroke:#d97706,color:#78350f
  class S0,S1,S2,S3,S4 act1
  class S5,S6,S7,S8,S9,S10 act2
```

<details>
<summary><b>The question each step answers</b></summary>

| Step | The question |
|---|---|
| **0** Baseline | Where does a plain setup break, and why? |
| **1** Queries | How much was just bad queries? |
| **2** Pooling | Are requests waiting on the database, or on a connection to it? |
| **3** Profiling | When does the Go side start to matter? |
| **4** Cache | Is a cache worth the CPU it steals from the database? |
| **5** Managed DB | What does the network hop cost, and what does "managed" buy? |
| **6** Read replica | What changes when reads and writes go different ways? |
| **7** Scale out | Does more API help, or just hit the database harder? |
| **8** Async writes | What do you trade for cheap writes? *(Kafka API on Azure Event Hubs)* |
| **9** Fan-out | Build feeds on read or on write — and what does a celebrity do to it? |
| **10** Overload | Past the limit, does it bend or break? |

</details>

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

**Built with** Go · Postgres · k6 · Grafana · Docker · Terraform · Ansible · Azure

---

Inspired by Arjay's [I Tested 8 Programming Languages on a $12 Server](https://www.youtube.com/watch?v=sQXFhh_PiG4) —
StressLab holds the language still and asks what else there is to squeeze.

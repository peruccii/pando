# Git Panel Performance Baseline

Data da medicao: **2026-02-23**

## Budgets alvo

Latencia (spec `git_specs/performance_reliability.md`):

- abrir painel Git (pipeline de abertura): **<= 300ms**
- primeira pagina de historico: **<= 200ms**
- `stage` arquivo pequeno/medio: **<= 150ms**
- `unstage` arquivo pequeno/medio: **<= 150ms**

Burst do watcher + responsividade (hardening P2):

- enqueue de burst (2k eventos) com coalescing: mediana **<= 120ms**
- flush do debounce de invalidação Git Panel: mediana **<= 320ms**

## Como medir

```bash
./scripts/gitpanel_perf_baseline.sh
```

O script executa:

- `TestGitPanelLatencyBudgets` (latencia end-to-end do dominio Git Panel em repo medio)
- `TestGitPanelWatcherBurstBudgets` (burst de watcher, coalescing e tempo de flush)

## Resultado baseline (local)

Execucao:

```bash
./scripts/gitpanel_perf_baseline.sh
```

Metricas observadas:

- `open_panel`: mediana **21.555667ms**, p95 **22.843458ms**
- `history_first_page`: mediana **24.377625ms**, p95 **26.41125ms**
- `stage_file`: mediana **13.702417ms**, p95 **13.862958ms**
- `unstage_file`: mediana **14.539334ms**, p95 **14.873792ms**
- `watcher_burst_enqueue` (2k eventos): mediana **116.625µs**, p95 **126.583µs**
- `watcher_burst_flush`: mediana **122.849167ms**, p95 **122.926875ms**

Status: **todos abaixo do budget**.

## Gate em CI

Workflow: `.github/workflows/gitpanel-smoke.yml`  
Script: `scripts/ci_gitpanel_smoke.sh`

Cobertura obrigatoria do gate:

- parser de status
- queue de write
- bindings do Git Panel
- budget de latencia do Git Panel
- budget de burst/responsividade do bridge watcher -> Git Panel
- build frontend

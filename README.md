# Deduplicação de Webhooks

Este repositório contém o testbed do artigo **Deduplicação de Webhooks de Pagamento
sob Concorrência e Falhas: um Estudo Experimental de Atomicidade**, de Lucas Gabriel
das Neves Moura, aceito na ERI-ES 2026.
Ele implementa um consumidor sintético de webhooks e compara três
formas de reivindicação de eventos sob reentrega sequencial, concorrência e
queda de processo entre duas escritas.

O código foi escrito especificamente para o estudo. Ele não contém código,
dados, nomes ou métricas de sistemas de produção.

## Requisitos

- Go 1.26.4;
- Docker com Docker Compose;
- portas locais 5433 e 8082 disponíveis.

O Compose usa usuário e senha `harness` exclusivamente no PostgreSQL local e
descartável do experimento. Essas credenciais não devem ser reutilizadas fora do
ambiente de teste.

## Estratégias

| Flag | Implementação |
|---|---|
| `none` | aceita toda entrega, sem deduplicação |
| `idem-key` | `SELECT` seguido de `INSERT`, sem atomicidade |
| `dedup` | `INSERT ... ON CONFLICT DO NOTHING` com unicidade |

Os nomes das flags são identificadores históricos. A variável experimental é a
atomicidade da reivindicação.

## Verificação rápida

```bash
go test -short ./...
make verify-results
make run-exp-a-min
make stop
```

O smoke test executa quatro células reduzidas e grava a saída em `/tmp`; ele não
altera os CSVs publicados.

## Reprodução

```bash
make infra-up
make run-exp-a-full
make run-exp-a-sensitivity
make stop
```

As novas execuções são gravadas como `results/reproduced-*`. Células sequenciais
e de crash são determinísticas. A corrida concorrente depende do escalonamento.
Compare a direção do resultado e a distribuição; os totais exatos podem variar.

O protocolo completo está em [EXPERIMENT.md](EXPERIMENT.md). As limitações estão
em [THREATS.md](THREATS.md). Os hashes dos resultados publicados estão em
`RESULTS.sha256`.

O campo `git_commit` em `results/exp-a-sensitivity-environment.txt` registra o HEAD
de um repositório de desenvolvimento anterior, com alterações ainda não commitadas
na execução da sensibilidade. Esse commit não contém o experimento completo e não
corresponde ao histórico deste repositório. O manifesto foi preservado como gerado;
o código e os resultados publicados aqui incluem o experimento de sensibilidade.

## Estrutura

```text
cmd/consumer/       sistema sob teste
cmd/loadgen/        gerador de carga e cenários
cmd/summarize/      agregação das execuções
internal/eventstore estratégias intercambiáveis
internal/fault/     ponto determinístico de crash
internal/oracle/    reconciliação por identidade
internal/metrics/   escrita e leitura dos CSVs
results/            dados publicados e manifesto de ambiente
```

## Citação

MOURA, Lucas Gabriel das Neves. **Deduplicação de Webhooks de Pagamento sob
Concorrência e Falhas: um Estudo Experimental de Atomicidade**. Artigo aceito na
ERI-ES 2026, 2026.

Os metadados do artefato e a referência ao artigo estão em [CITATION.cff](CITATION.cff).
Repositório: <https://github.com/lgsn0w/deduplicacao-de-webhooks>.

## Licença

Código disponibilizado sob a licença MIT.

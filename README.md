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

| Estratégia no artigo | Flag | Implementação |
|---|---|---|
| Sem controle | `none` | aceita toda entrega, sem deduplicação |
| Reivindicação não atômica | `idem-key` | consulta e tenta inserir; decide pela consulta anterior |
| Reivindicação atômica | `dedup` | insere com unicidade; prossegue apenas se inseriu |

Os nomes das flags são identificadores históricos. A variável experimental é a
atomicidade da reivindicação.
Ambas as estratégias com marcador usam `INSERT ... ON CONFLICT DO NOTHING`;
somente a reivindicação atômica usa o resultado da inserção para decidir se processa.

## Testes

1. Testes rápidos, sem banco:

   ```bash
   go test -short -count=1 ./...
   ```

2. Suíte completa em Docker, com PostgreSQL exclusivo, sem portas expostas ou montagem de pastas locais:

   ```bash
   docker compose -p eri-tests -f compose.test.yml up --build --abort-on-container-exit --exit-code-from tests
   ```

3. Remover o ambiente de testes, inclusive se algum teste falhar:

   ```bash
   docker compose -p eri-tests -f compose.test.yml down --volumes
   ```

Os testes não alteram os CSVs publicados nem usam o banco do experimento.

## Verificação dos resultados e execução mínima

```bash
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

A matriz principal cruza três estratégias, três cenários e cinco sementes (1 a 5):
45 execuções. Reentrega e concorrência usam 1.000 eventos; queda usa 100.
São dez entregas por evento na concorrência. A sensibilidade cruza `fresh` (sem
reúso de conexões) e `pooled` (com reúso), `idem-key` e `dedup`, com cinco sementes
e 1.000 eventos por execução: mais 20 execuções.

`timeout` designa reentrega sequencial após sucesso, sem falha de rede injetada.
O efeito é uma escrita em `processing_log`, separada do marcador em `event_log`;
a queda é provocada entre essas etapas. A carga é sintética e não estima taxas
de falha em produção. Transação local, outbox e publicação externa não são avaliados.
Os hashes dos resultados publicados estão em `RESULTS.sha256`.

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

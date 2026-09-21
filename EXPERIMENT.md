# Protocolo experimental

## Matriz principal

- estratégias: `none`, `idem-key`, `dedup`;
- cenários: `timeout`, `concurrent`, `crash`;
- sementes: 1, 2, 3, 4 e 5;
- reentrega e concorrência: 1.000 identidades por execução;
- concorrência: dez entregas simultâneas por identidade;
- crash: 100 identidades por execução.

`timeout` é o identificador histórico da reentrega sequencial após sucesso. O
testbed envia duas requisições e não injeta nem mede timeout de rede.

No cenário `crash`, o consumidor encerra depois do commit em `event_log` e antes
da escrita em `processing_log`. O Docker reinicia o processo e o gerador envia a
retentativa.

## Sensibilidade ao transporte

A matriz adicional usa:

- transportes `fresh` e `pooled`;
- estratégias `idem-key` e `dedup`;
- cinco sementes;
- 1.000 identidades e dez entregas concorrentes por identidade.

`fresh` desativa keep-alive; `pooled` reutiliza conexões.

## Métricas

O oráculo reconcilia eventos esperados e observados por identidade:

- duplicata: observações excedem o esperado;
- perdido: identidade esperada nunca observada;
- fantasma: identidade observada sem envio registrado.

## Resultados esperados para validação

| Estratégia | Reentrega | Concorrência | Crash |
|---|---|---|---|
| `none` | duplica | duplica | recupera |
| `idem-key` | protege | duplica | perde |
| `dedup` | protege | protege | perde |

Os resultados concorrentes não são bit a bit reproduzíveis porque medem uma
corrida real. Médias, desvios e extremos devem ser recalculados para cada coleta.

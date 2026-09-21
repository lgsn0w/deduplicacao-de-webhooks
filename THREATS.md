# Limitações do experimento

- A carga é sintética e uniforme; não estima taxas de falha em produção.
- A implementação usa Go e PostgreSQL. Os mecanismos são gerais, mas as
  magnitudes dependem do ambiente.
- Identidades e cargas úteis são determinadas pelas sementes; o escalonamento de
  goroutines e conexões não é determinístico.
- `processing_log` é um efeito observável no mesmo banco, usado como modelo de
  uma segunda etapa não atômica. O experimento não implementa publicação externa.
- Uma transação local poderia tornar as duas tabelas concretas atômicas. Outbox e
  outros ciclos recuperáveis não são avaliados.
- O crash é injetado em ponto de código determinístico e modela uma queda entre
  escritas; não cobre todos os modos de falha de processo ou infraestrutura.

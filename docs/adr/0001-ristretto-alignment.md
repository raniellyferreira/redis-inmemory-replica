# ADR-0001: Alinhamento do Storage aos Princípios do Ristretto

Status: Proposta
Data: 2025-10-11
Autores: @raniellyferreira

## Contexto

O storage atual provê TTL/expiração, limpeza incremental por amostragem e uma evicção LRU simplificada. Para cargas intensas e melhor taxa de acerto, queremos alinhar o design aos princípios do Ristretto.

## Objetivos

- Preparar uma arquitetura pluggable de políticas de admissão e evicção
- Introduzir no design o conceito de custo por item
- Manter a limpeza de expirados (TTL) como responsabilidade separada da admissão/evicção
- Preservar compatibilidade e estabilidade com alteração incremental

## Princípios Ristretto a considerar [Unverified]

- Admissão TinyLFU para filtrar itens raros antes de entrar no cache
- Evicção por amostragem/frequência e custo por item
- Ênfase em throughput e baixa contenção de locks

## Estado Atual (Resumo)

- Evicção: função simplificada `evictLRU` (storage/memory.go)
- Cleanup de TTL: amostragem com rounds e batches (Storage Cleanup Configurations) – já robusta
- Medição: `MemoryUsage`, `KeyCount` e algumas benches de cleanup; falta foco em hit ratio

## Decisões Propostas

1. Introduzir interfaces de política (admissão/evicção/custo) em pacote `storage/policy` sem alterar o runtime atual
2. Fornecer implementações placeholder (NoopAdmission, PlaceholderLRU) para compilação e evolução
3. Definir contrato de custo simples e extensível (p.ex. custo = size por padrão), delegando especializações futuras
4. Adicionar benchmarks esqueleto orientados a comparar caminhos atuais vs. futuros (skipped no curto prazo)

## Alternativas Consideradas

- Acoplar diretamente TinyLFU ao storage: risco de alto impacto; rejeitado nesta etapa
- Usar uma lib externa: avaliar em fase seguinte; manter primeiro a arquitetura pluggable

## Plano Incremental

**Fase 1 (esta PR)**: ADR + scaffolding + benches esqueleto
**Fase 2**: Protótipo de TinyLFU [Unverified] + sampled eviction atrás de feature flag
**Fase 3**: Integração opcional com MemoryStorage + métricas e validações (hit ratio/latência)
**Fase 4**: Endurecimento, tuning e documentação final

## Critérios de Sucesso (alto nível)

- Hit ratio e latência iguais ou melhores que baseline sob perfis comuns
- Uso de memória previsível sob pressão
- Baixa contenção (medida via benchmarks de concorrência)

## Riscos

- Complexidade de sincronização e overhead de estruturas probabilísticas [Unverified]
- Regressões de latência no caminho GET/SET

## Referências

- Ristretto (github.com/hypermodeinc/ristretto/v2) [Unverified]
- TinyLFU (artigo/padrões) [Unverified]

## Seguimento

Issues serão abertas para cada fase (admissão, evicção, custo, integração, benchmarks).

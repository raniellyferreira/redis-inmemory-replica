---
name: skill-task-orchestrator
description: >
  Orquestra o desenvolvimento sequencial de tasks (issues) utilizando subagents.
  Fluxo: update main → create branch → develop → commit/push → open PR → aguardar review.
  Use quando precisar executar uma lista de issues/tasks de forma organizada,
  uma por vez, com PRs individuais para cada task.
---

# Skill: Task Development Orchestrator

Você é um orquestrador de desenvolvimento que executa tasks (issues do GitHub)
de forma sequencial e disciplinada, delegando o trabalho pesado para subagents
especializados e seguindo um fluxo Git rigoroso.

---

## Pré-requisitos

Antes de iniciar, verifique:

1. **Repositório limpo**: `git status` não deve ter mudanças não commitadas
2. **Branch main**: Você deve estar na branch `main` ou ser capaz de voltar a ela
3. **Remote configurado**: `origin` deve apontar para o repositório correto
4. **Issues existentes**: As tasks devem existir como issues no GitHub
5. **Tabela SQL `todos`**: Se houver uma tabela de todos na sessão, use-a para
   rastrear progresso. Caso contrário, crie uma.

---

## Fluxo por Task

Para **cada task**, siga este fluxo **exatamente nesta ordem**:

### Fase 1 — Preparação

```
1. Consulte a próxima task pronta (sem dependências pendentes):
   SELECT t.* FROM todos t
   WHERE t.status = 'pending'
   AND NOT EXISTS (
       SELECT 1 FROM todo_deps td
       JOIN todos dep ON td.depends_on = dep.id
       WHERE td.todo_id = t.id AND dep.status != 'done'
   )
   LIMIT 1;

2. Se não houver task pronta, PARE e informe ao usuário.

3. Atualize o status para in_progress:
   UPDATE todos SET status = 'in_progress' WHERE id = '<task_id>';

4. Informe ao usuário qual task será executada, com título e resumo.
```

### Fase 2 — Git Setup

```
1. Volte para a branch main:
   git checkout main

2. Atualize a main com o remote:
   git pull origin main

3. Crie uma nova branch a partir da main:
   git checkout -b <branch_name>

   Convenção de nomes:
   - Bug fix:     fix/<issue_number>-<slug>
   - Feature:     feat/<issue_number>-<slug>
   - Refactoring: refactor/<issue_number>-<slug>
   - Architecture: arch/<issue_number>-<slug>

   Exemplo: refactor/774-decompose-orchestrator-god-object
```

### Fase 3 — Desenvolvimento

```
1. Leia a issue completa no GitHub (título, body, labels) para entender o escopo.

2. Analise o código relevante no repositório local:
   - Identifique os arquivos que precisam ser alterados
   - Entenda as dependências e impactos
   - Verifique testes existentes

3. Delegue o desenvolvimento para o subagent mais apropriado:
   - "Principal Software Architect" → Para refatorações arquiteturais (#774, #775, #777-#780)
   - "Software Engineer" → Para bug fixes e implementações (#766, #770, #771, #776)
   - "Senior Product Engineer (Frontend)" → Para mudanças no MiniApp (/web)

4. O subagent DEVE:
   - Implementar a solução completa
   - Adicionar/atualizar testes unitários
   - Manter o código limpo (SRP, DRY, KISS)
   - Não quebrar funcionalidades existentes

5. Após o desenvolvimento, execute:
   - make fmt        (formatação — OBRIGATÓRIO)
   - make lint       (linting)
   - make test-unit  (testes unitários)

6. Se houver falhas, corrija-as antes de prosseguir.
   - Se o subagent falhar repetidamente, assuma o trabalho diretamente.
```

### Fase 4 — Commit, Push e PR

```
1. Adicione as mudanças:
   git add -A

2. Faça o commit seguindo conventional commits:
   git commit -m "<type>(agent-teams): <descrição concisa>

   <corpo detalhado do que foi feito>

   Closes #<issue_number>

   Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"

   Tipos: fix, feat, refactor, test, docs, chore

3. Push para o remote:
   git push -u origin <branch_name>

4. Abra o Pull Request via GitHub API:
   - Título: O mesmo do commit principal
   - Body: Descrição detalhada com:
     • O que foi alterado e por quê
     • Lista de arquivos modificados com resumo
     • Como testar
     • Link para a issue (Closes #XXX)
   - Base: main
   - Labels: mesmos da issue
   - Draft: false (pronto para review)
```

### Fase 5 — Aguardar Review

```
1. Informe ao usuário:
   "✅ PR #XXX criado para a task #YYY: <título>
    🔗 <url_do_pr>
    ⏳ Aguardando seu review antes de prosseguir para a próxima task."

2. PARE e aguarde o usuário responder.
   - Se o usuário aprovar: marque a task como done e prossiga para a próxima.
   - Se o usuário pedir mudanças: aplique as correções no mesmo branch,
     commit + push, e aguarde novamente.
   - Se o usuário pedir para pular: marque como blocked e prossiga.

3. Atualize o status:
   UPDATE todos SET status = 'done' WHERE id = '<task_id>';
```

---

## Regras Importantes

### Git Hygiene
- **NUNCA** faça commit direto na `main`
- **NUNCA** force push (`--force`)
- Cada task = 1 branch = 1 PR
- Branch names devem ser descritivos e incluir o número da issue

### Qualidade
- Testes DEVEM passar antes de abrir PR
- `make fmt` e `make lint` DEVEM estar limpos
- Não introduza regressões

### Comunicação
- Sempre informe ao usuário antes de iniciar cada task
- Sempre informe o resultado (PR criado ou erro encontrado)
- Em caso de dúvida sobre escopo, pergunte ao usuário

### Dependências entre Tasks
- Respeite o grafo de dependências na tabela `todo_deps`
- Só execute tasks cujas dependências estejam com status `done`
- Se uma task estiver bloqueada, pule para a próxima disponível

### Erro e Recuperação
- Se um subagent falhar, tente uma vez mais com contexto adicional
- Se falhar novamente, faça o trabalho diretamente
- Se a task se mostrar inviável, marque como `blocked` com motivo e siga adiante

---

## Resumo do Ciclo

```
┌─────────────────────────────────────────────────────────┐
│  LOOP (para cada task pronta no backlog):                │
│                                                          │
│  1. 📋 Query próxima task sem dependências pendentes     │
│  2. 🔄 git checkout main && git pull origin main         │
│  3. 🌿 git checkout -b <type>/<issue>-<slug>            │
│  4. 🛠️  Delegar desenvolvimento ao subagent apropriado   │
│  5. ✅ make fmt && make lint && make test-unit           │
│  6. 📦 git add -A && git commit && git push             │
│  7. 🔀 Abrir PR via GitHub API                          │
│  8. ⏸️  PARAR e aguardar review do usuário               │
│  9. ✅ Marcar task como done                             │
│  └──→ Voltar ao passo 1                                  │
└─────────────────────────────────────────────────────────┘
```

---

## Exemplo de Uso

O usuário pode invocar esta skill dizendo:

- "Execute a próxima task do backlog"
- "Comece o desenvolvimento das issues de Agent Teams"
- "Rode o skill-task-orchestrator para as issues #766 a #780"

A skill então consulta o backlog, identifica a próxima task pronta,
e inicia o ciclo descrito acima.

# Spec: Observability and Security

## 1. Objetivo

Garantir transparencia operacional e seguranca na execucao de comandos Git locais.

## 2. Observabilidade

Cada comando Git deve gerar evento de diagnostico com:

- `commandId`
- `repoPath`
- `action`
- `args (sanitized)`
- `durationMs`
- `exitCode`
- `stderrSanitized`
- `status`

Logs devem permitir correlacao entre:

- acao do usuario
- comando executado
- refresh do estado no painel

## 3. Console de Saida (Opcional)

- mostrar comando real executado (sem segredos)
- mostrar tempo e resultado
- manter historico curto rotativo

## 4. Guardrails de Seguranca

- validar `repoPath` existe e e repo Git
- validar `filePath` e relativo ao repo
- bloquear traversal (`..`) e caminhos fora do repo
- executar comandos com argumentos estruturados (sem shell string concatenada)
- usar `--` antes de paths quando suportado

## 5. Privacidade

- nenhuma telemetria de codigo para terceiros
- dados de repositorio local ficam no host
- logs nao devem armazenar conteudo sensivel completo

## 6. Tratamento de Erros Sensiveis

- sanitizar stderr antes de exibir na UI
- evitar dump de path absoluto sensivel quando nao necessario

## 7. Criterios de Aceite

- comandos invalidos por path sao bloqueados com erro explicito
- logs permitem diagnosticar falha sem expor dados desnecessarios
- comportamento consistente com politica local-first do ORCH


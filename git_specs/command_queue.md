# Spec: Sequential Command Queue

## 1. Objetivo

Evitar conflitos de concorrencia no Git (`index.lock` e estados inconsistentes), serializando comandos write por repositorio.

## 2. Escopo

Entram na fila:

- stage/unstage/discard
- apply patch parcial
- accept mine/theirs
- demais comandos que alteram index/working tree/refs locais

Nao entram:

- comandos apenas de leitura (`log`, `status`, `diff`)

## 3. Design

- uma fila por `repoPath`
- um worker goroutine por fila
- processamento FIFO
- cada item contem:
  - command id
  - tipo da acao
  - argumentos
  - timeout
  - tentativa atual

## 4. Retry Policy

- retries apenas para erros transientes de lock
- policy recomendada:
  - max 3 tentativas
  - backoff: 80ms, 160ms, 320ms
- erro final exposto ao frontend com contexto tecnico resumido

## 5. Cancelamento

- comandos de usuario devem aceitar cancelacao por contexto
- comandos em andamento nao devem bloquear shutdown indefinidamente

## 6. Observabilidade

Emitir evento por comando:

- queued
- started
- retried
- succeeded
- failed

Campos minimos:

- repo path
- command type
- duration ms
- exit code
- sanitized stderr

## 7. Criterios de Aceite

- duas operacoes write simultaneas no mesmo repo nao colidem
- erros `index.lock` caem na policy de retry
- UI recebe resultado deterministico por acao


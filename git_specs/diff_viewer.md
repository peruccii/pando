# Spec: Diff Viewer

## 1. Objetivo

Exibir diffs com clareza e performance nos modos unified e side-by-side, com foco em leitura e operacao rapida.

## 2. Requisitos Funcionais

- alternar entre modo unified e split
- mostrar diffs por arquivo com hunks colapsaveis
- highlight de sintaxe para JS/TS, Go, Rust e Python
- sincronizacao de scroll no modo split
- suporte a navegar por teclado entre arquivos e hunks

## 3. Pipeline de Dados

- backend retorna modelo estruturado de diff (arquivos, hunks, linhas)
- frontend renderiza com lazy mount por arquivo
- arquivos fora da viewport nao devem manter renderizacao pesada

## 4. Performance e Degradacao

- arquivo > 1MB:
  - sem preview automatico
  - mostrar CTA "Load anyway"
- arquivo binario:
  - nao renderizar texto
  - mostrar metadado de arquivo binario
- limite de linhas por render batch para evitar frame drops

## 5. Scroll Sync

- lock-step opcional no modo split
- origem do scroll define alvo (evitar loop de eventos)
- tolerancia para diferenca de altura entre lados

## 6. Criterios de Aceite

- navegacao de diffs sem congelar UI
- side-by-side sincronizado de forma previsivel
- fallback correto para binarios e arquivos grandes

## 7. Erros e Recovery

- erro de parser: fallback para raw diff texto
- erro de highlight: render plain text mantendo navegacao


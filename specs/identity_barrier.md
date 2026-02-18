# Spec: Barreira de Identidade (Identity Barrier)

> **Módulo**: Transversal — Auth UX  
> **Status**: Draft  
> **PRD Ref**: Seção 7.5  
> **Última Atualização**: 12 de Fevereiro de 2026

---

## 1. Objetivo

Implementar **"Progressive Disclosure"** baseado em autenticação. O usuário pode entrar para observar (Read-Only) sem login, mas para agir no GitHub, o app exige autenticação. A UI deve ser **reativa à identidade**.

---

## 2. Estado Global de Autenticação

```typescript
interface AuthState {
    isAuthenticated: boolean
    user: User | null
    githubToken: string | null  // Em memória, nunca persistido inseguro
}

interface User {
    id: string
    login: string       // GitHub username
    name: string
    email: string
    avatarUrl: string
    provider: 'github' | 'google'
}
```

---

## 3. Comportamento da UI por Estado

### 3.1 Autenticado (`isAuthenticated = true`)

| Elemento              | Comportamento                  |
| ---------------------- | ------------------------------ |
| Botão "Criar PR"       | ✅ Habilitado, cor primária     |
| Botão "Comentar"       | ✅ Habilitado                   |
| Botão "Aprovar/Merge"  | ✅ Habilitado                   |
| Botão "Criar Issue"    | ✅ Habilitado                   |
| Terminal (sessão P2P)  | ✅ Habilitado                   |
| Avatar no header       | Foto do usuário + dropdown     |

### 3.2 Não Autenticado (`isAuthenticated = false`)

| Elemento              | Comportamento                                   |
| ---------------------- | ------------------------------------------------ |
| Botão "Criar PR"       | 🔒 Disabled + tooltip "Faça login no GitHub"     |
| Botão "Comentar"       | 🔒 Disabled + tooltip                            |
| Botão "Aprovar/Merge"  | 🔒 Disabled + tooltip                            |
| Botão "Criar Issue"    | 🔒 Disabled + tooltip                            |
| Terminal (sessão P2P)  | ✅ Habilitado (se sessão permite anônimos)        |
| Visualizar PRs/Diffs   | ✅ Habilitado (read-only via Host proxy)          |
| Avatar no header       | Ícone genérico + botão "Login"                   |

---

## 4. Componente `AuthGuard`

```typescript
interface AuthGuardProps {
    children: React.ReactNode
    fallback?: React.ReactNode       // O que mostrar se não autenticado
    action?: string                  // "Criar PR", "Comentar", etc.
    requireGitHub?: boolean          // Requer GitHub especificamente?
}

function AuthGuard({ children, fallback, action, requireGitHub }: AuthGuardProps) {
    const { isAuthenticated, user } = useAuth()

    if (!isAuthenticated) {
        if (fallback) return <>{fallback}</>

        return (
            <button
                disabled
                className="btn--disabled"
                title={`Faça login no GitHub para ${action || 'realizar esta ação'}`}
            >
                🔒 {action || 'Login necessário'}
            </button>
        )
    }

    if (requireGitHub && user?.provider !== 'github') {
        return (
            <button
                onClick={() => auth.linkGitHub()}
                className="btn--warning"
            >
                Conectar GitHub para {action}
            </button>
        )
    }

    return <>{children}</>
}
```

### 4.1 Uso

```tsx
{/* Botão que requer autenticação */}
<AuthGuard action="Criar PR" requireGitHub>
    <button onClick={handleCreatePR}>
        + Criar Pull Request
    </button>
</AuthGuard>

{/* Área read-only (sem guard) */}
<PRDiffViewer diff={diff} />

{/* Comentário inline (com guard) */}
<AuthGuard action="Comentar" requireGitHub>
    <InlineCommentInput onSubmit={handleComment} />
</AuthGuard>
```

---

## 5. Login Prompt Contextual

Quando um usuário não autenticado clica em um botão protegido:

```
┌──────────────────────────────────────────┐
│  🔐 Login necessário                    │
│                                          │
│  Para criar um Pull Request, você        │
│  precisa estar conectado ao GitHub.      │
│                                          │
│  [  Login com GitHub  ]                 │
│  [  Login com Google  ]                 │
│                                          │
│  [ Continuar sem login (apenas leitura)] │
└──────────────────────────────────────────┘
```

---

## 6. CSS

```css
/* Botão desabilitado por falta de auth */
.btn--auth-required {
    position: relative;
    opacity: 0.5;
    cursor: not-allowed;
}

.btn--auth-required::after {
    content: "🔒";
    position: absolute;
    top: -4px;
    right: -4px;
    font-size: 10px;
}

.btn--auth-required:hover {
    opacity: 0.7;
}

/* Tooltip */
.btn--auth-required[title]:hover::before {
    content: attr(title);
    position: absolute;
    bottom: 100%;
    left: 50%;
    transform: translateX(-50%);
    padding: 4px 8px;
    background: var(--bg-tooltip);
    color: var(--fg-tooltip);
    border-radius: 4px;
    font-size: 11px;
    white-space: nowrap;
    z-index: 1000;
}
```

---

## 7. Dependências

| Dependência              | Tipo       | Spec Relacionada       |
| ------------------------- | ---------- | ---------------------- |
| AuthService (OAuth)       | Bloqueador | auth_and_persistence   |
| zustand (auth store)      | Bloqueador | command_center_ui      |

# Spec: Design System & Temas

> **Módulo**: Transversal — UI Foundation  
> **Status**: Draft  
> **PRD Ref**: Seção 11, 14  
> **Última Atualização**: 12 de Fevereiro de 2026

---

## 1. Objetivo

Definir o sistema de design (tokens, tipografia, cores, espaçamento) e os temas do ORCH (Dark, Light, Hacker, Nvim, Min Dark). Garantir consistência visual entre todos os módulos.

---

## 2. Temas

### 2.1 Dark (Padrão)

```css
:root[data-theme="dark"] {
    /* Background */
    --bg-primary: #0f0f14;
    --bg-secondary: #16161e;
    --bg-tertiary: #1a1b26;
    --bg-elevated: #1e2030;
    --bg-header: #13131a;
    --bg-hover: rgba(255, 255, 255, 0.06);
    --bg-active: rgba(255, 255, 255, 0.1);

    /* Foreground */
    --fg-primary: #c0caf5;
    --fg-secondary: #9aa5ce;
    --fg-muted: #565f89;
    --fg-inverse: #0f0f14;

    /* Accent */
    --accent: #7aa2f7;
    --accent-hover: #89b4fa;
    --accent-rgb: 122, 162, 247;

    /* Semantic */
    --color-success: #9ece6a;
    --color-warning: #e0af68;
    --color-error: #f7768e;
    --color-info: #7dcfff;

    /* Borders */
    --border-primary: #292e42;
    --border-subtle: #1e2030;
    --border-focus: var(--accent);

    /* Shadows */
    --shadow-sm: 0 1px 2px rgba(0, 0, 0, 0.3);
    --shadow-md: 0 4px 12px rgba(0, 0, 0, 0.4);
    --shadow-glow: 0 0 12px rgba(var(--accent-rgb), 0.3);
}
```

### 2.2 Light

```css
:root[data-theme="light"] {
    --bg-primary: #fafafa;
    --bg-secondary: #f0f0f0;
    --bg-tertiary: #e8e8e8;
    --bg-elevated: #ffffff;
    --bg-header: #f5f5f5;
    --bg-hover: rgba(0, 0, 0, 0.04);
    --bg-active: rgba(0, 0, 0, 0.08);

    --fg-primary: #383a42;
    --fg-secondary: #696c77;
    --fg-muted: #a0a1a7;
    --fg-inverse: #ffffff;

    --accent: #4078f2;
    --accent-hover: #526fff;
    --accent-rgb: 64, 120, 242;

    --color-success: #50a14f;
    --color-warning: #c18401;
    --color-error: #e45649;
    --color-info: #0184bc;

    --border-primary: #d4d4d8;
    --border-subtle: #e4e4e7;
    --border-focus: var(--accent);
}
```

### 2.3 Hacker

```css
:root[data-theme="hacker"] {
    --bg-primary: #0a0a0a;
    --bg-secondary: #0d0d0d;
    --bg-tertiary: #111111;
    --bg-elevated: #151515;
    --bg-header: #080808;
    --bg-hover: rgba(0, 255, 65, 0.06);
    --bg-active: rgba(0, 255, 65, 0.1);

    --fg-primary: #00ff41;
    --fg-secondary: #00cc33;
    --fg-muted: #006622;
    --fg-inverse: #0a0a0a;

    --accent: #00ff41;
    --accent-hover: #33ff66;
    --accent-rgb: 0, 255, 65;

    --color-success: #00ff41;
    --color-warning: #ffcc00;
    --color-error: #ff0040;
    --color-info: #00ccff;

    --border-primary: #1a3a1a;
    --border-subtle: #0f1f0f;
    --border-focus: var(--accent);
}
```

### 2.4 Nvim (Old School)

```css
:root[data-theme="nvim"] {
    --bg-primary: #121212;
    --bg-secondary: #1a1a1a;
    --bg-tertiary: #202020;
    --bg-elevated: #262626;

    --fg-primary: #d4cfbf;
    --fg-secondary: #b5ae99;
    --fg-muted: #8c8678;

    --accent: #d19a66;
    --accent-hover: #e0ad7b;
    --accent-rgb: 209, 154, 102;

    --color-success: #98c379;
    --color-warning: #e5c07b;
    --color-error: #e06c75;
    --color-info: #61afef;

    --border-primary: #3a352c;
    --border-subtle: #2c2822;
    --border-focus: var(--accent);
}
```

### 2.5 Min Dark (Minimalista, sem bordas)

```css
:root[data-theme="min-dark"] {
    --bg-primary: #0f1115;
    --bg-secondary: #13161b;
    --bg-tertiary: #171b22;
    --bg-elevated: #1b2028;

    --fg-primary: #e5e7eb;
    --fg-secondary: #b6bcc8;
    --fg-muted: #8a93a3;

    --accent: #93c5fd;
    --accent-hover: #a8d1ff;
    --accent-rgb: 147, 197, 253;

    --color-success: #86efac;
    --color-warning: #fcd34d;
    --color-error: #fda4af;
    --color-info: #93c5fd;

    --border-primary: transparent;
    --border-subtle: transparent;
    --border-focus: transparent;
}
```

---

## 3. Tipografia

```css
:root {
    /* Font families */
    --font-sans: 'Inter', -apple-system, BlinkMacSystemFont, sans-serif;
    --font-mono: 'JetBrains Mono', 'Fira Code', 'SF Mono', monospace;

    /* Font sizes */
    --text-xs: 11px;
    --text-sm: 12px;
    --text-base: 14px;
    --text-lg: 16px;
    --text-xl: 18px;
    --text-2xl: 24px;

    /* Font weights */
    --font-normal: 400;
    --font-medium: 500;
    --font-semibold: 600;
    --font-bold: 700;

    /* Line heights */
    --leading-tight: 1.25;
    --leading-normal: 1.5;
    --leading-relaxed: 1.75;
}
```

---

## 4. Espaçamento

```css
:root {
    --space-1: 4px;
    --space-2: 8px;
    --space-3: 12px;
    --space-4: 16px;
    --space-5: 20px;
    --space-6: 24px;
    --space-8: 32px;
    --space-10: 40px;
    --space-12: 48px;

    /* Border radius */
    --radius-sm: 4px;
    --radius-md: 6px;
    --radius-lg: 8px;
    --radius-xl: 12px;
    --radius-full: 9999px;
}
```

---

## 5. Componentes Base

### 5.1 Botões

```css
.btn {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-4);
    border-radius: var(--radius-md);
    font-size: var(--text-sm);
    font-weight: var(--font-medium);
    font-family: var(--font-sans);
    cursor: pointer;
    border: 1px solid transparent;
    transition: all 0.15s ease;
}

.btn--primary {
    background: var(--accent);
    color: var(--fg-inverse);
}

.btn--primary:hover {
    background: var(--accent-hover);
}

.btn--ghost {
    background: transparent;
    color: var(--fg-secondary);
}

.btn--ghost:hover {
    background: var(--bg-hover);
    color: var(--fg-primary);
}

.btn--danger {
    background: var(--color-error);
    color: white;
}

.btn:disabled {
    opacity: 0.4;
    cursor: not-allowed;
}
```

### 5.2 Inputs

```css
.input {
    width: 100%;
    padding: var(--space-2) var(--space-3);
    background: var(--bg-secondary);
    border: 1px solid var(--border-primary);
    border-radius: var(--radius-md);
    color: var(--fg-primary);
    font-size: var(--text-sm);
    font-family: var(--font-sans);
    transition: border-color 0.15s ease;
}

.input:focus {
    outline: none;
    border-color: var(--accent);
    box-shadow: 0 0 0 2px rgba(var(--accent-rgb), 0.2);
}
```

### 5.3 Badges

```css
.badge {
    display: inline-flex;
    align-items: center;
    padding: 2px 8px;
    border-radius: var(--radius-full);
    font-size: var(--text-xs);
    font-weight: var(--font-medium);
}

.badge--success { background: rgba(158, 206, 106, 0.15); color: var(--color-success); }
.badge--warning { background: rgba(224, 175, 104, 0.15); color: var(--color-warning); }
.badge--error   { background: rgba(247, 118, 142, 0.15); color: var(--color-error); }
.badge--info    { background: rgba(125, 207, 255, 0.15); color: var(--color-info); }
```

---

## 6. Animações

```css
/* Micro-animações */
@keyframes fadeIn {
    from { opacity: 0; transform: translateY(4px); }
    to   { opacity: 1; transform: translateY(0); }
}

@keyframes pulse {
    0%, 100% { opacity: 1; }
    50%      { opacity: 0.4; }
}

@keyframes slideUp {
    from { transform: translateY(100%); opacity: 0; }
    to   { transform: translateY(0); opacity: 1; }
}

@keyframes glow {
    0%, 100% { box-shadow: 0 0 8px rgba(var(--accent-rgb), 0.2); }
    50%      { box-shadow: 0 0 16px rgba(var(--accent-rgb), 0.4); }
}

.animate-fade-in    { animation: fadeIn 0.2s ease; }
.animate-pulse      { animation: pulse 1.5s ease-in-out infinite; }
.animate-slide-up   { animation: slideUp 0.3s ease-out; }
.animate-glow       { animation: glow 2s ease-in-out infinite; }
```

---

## 7. Google Fonts

Incluir no `index.html`:

```html
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&family=JetBrains+Mono:wght@400;500;600&display=swap" rel="stylesheet">
```

---

## 8. Dependências

| Dependência       | Tipo       |
| ------------------ | ---------- |
| Google Fonts (CDN) | Opcional   |
| CSS Custom Props   | Nativo     |

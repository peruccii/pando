import { useState, useEffect, useRef, useMemo } from "react";
import {
    Check,
    Lock,
    Plus,
    Package,
    Terminal,
    Code2,
    Server,
    Search,
} from "lucide-react";
import * as App from "../../wailsjs/go/main/App";
import { useStackBuildStore } from "../stores/stackBuildStore";
import "./StackBuilder.css";

interface Tool {
    id: string;
    name: string;
    defaultVersion?: string;
    supportedVersions?: string[];
    category: "essential" | "language" | "tool";
    isLocked?: boolean;
}

const AVAILABLE_TOOLS: Tool[] = [
    // Essenciais
    { id: "git", name: "Git", category: "essential", isLocked: true },
    { id: "curl", name: "Curl", category: "essential", isLocked: true },
    { id: "wget", name: "Wget", category: "essential", isLocked: true },

    // Linguagens
    {
        id: "node",
        name: "Node.js",
        defaultVersion: "20",
        supportedVersions: ["18", "20", "22"],
        category: "language",
    },
    {
        id: "python",
        name: "Python",
        defaultVersion: "3.11",
        category: "language",
    },
    {
        id: "go",
        name: "Go",
        defaultVersion: "1.22",
        supportedVersions: ["1.21", "1.22", "1.23"],
        category: "language",
    },
    {
        id: "rust",
        name: "Rust",
        defaultVersion: "Stable",
        category: "language",
    },

    // Ferramentas
    { id: "ffmpeg", name: "FFmpeg", category: "tool" },
    { id: "jq", name: "JQ", category: "tool" },
    { id: "aws", name: "AWS CLI", defaultVersion: "v2", category: "tool" },
    { id: "docker-cli", name: "Docker CLI", category: "tool" },
];

export function StackBuilder() {
    // State local (apenas seleção de ferramentas e status de "já up-to-date")
    const [selectedTools, setSelectedTools] = useState<Record<string, string>>({
        git: "latest",
        curl: "latest",
        wget: "latest",
    });
    const [filterText, setFilterText] = useState("");
    const [isSuccess, setIsSuccess] = useState(false);
    const [asciiFrame, setAsciiFrame] = useState(0);

    // State global persistido (sobrevive à navegação)
    const isBuilding = useStackBuildStore((s) => s.isBuilding);
    const logs = useStackBuildStore((s) => s.logs);
    const elapsedTime = useStackBuildStore((s) => s.elapsedTime);
    const buildResult = useStackBuildStore((s) => s.result);
    const startBuild = useStackBuildStore((s) => s.startBuild);

    const logsEndRef = useRef<HTMLDivElement>(null);

    const asciiFrames = ["⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"];

    const estimatedTotalSeconds = useMemo(() => {
        const customCount = Object.keys(selectedTools).filter(id => {
            const tool = AVAILABLE_TOOLS.find(t => t.id === id);
            return tool && tool.category !== "essential";
        }).length;
        return 30 + (customCount * 15);
    }, [selectedTools]);

    // Animação do loader ASCII (apenas quando building)
    useEffect(() => {
        if (!isBuilding) return;

        const frameTimer = setInterval(() => {
            setAsciiFrame(prev => (prev + 1) % asciiFrames.length);
        }, 80);

        return () => clearInterval(frameTimer);
    }, [isBuilding]);

    const formatTime = (seconds: number) => {
        const mins = Math.floor(seconds / 60);
        const secs = seconds % 60;
        return `${mins}:${secs.toString().padStart(2, "0")}`;
    };

    const filteredTools = useMemo(() => {
        if (!filterText) return AVAILABLE_TOOLS;
        const lowerFilter = filterText.toLowerCase();
        return AVAILABLE_TOOLS.filter((t) =>
            t.name.toLowerCase().includes(lowerFilter),
        );
    }, [filterText]);

    // Scroll automático dos logs
    useEffect(() => {
        logsEndRef.current?.scrollIntoView({ behavior: "smooth" });
    }, [logs]);

    // Sincronizar resultado do build com isSuccess local
    useEffect(() => {
        if (buildResult === "success") {
            setIsSuccess(true);
        } else if (buildResult === "error") {
            setIsSuccess(false);
        }
    }, [buildResult]);

    // Load saved config
    useEffect(() => {
        const loadConfig = async () => {
            try {
                // @ts-ignore
                const savedTools: Record<string, string> = await App.GetCustomStackTools();
                if (savedTools && Object.keys(savedTools).length > 0) {
                    const merged = { ...savedTools };
                    if (!merged["git"]) merged["git"] = "latest";
                    if (!merged["curl"]) merged["curl"] = "latest";
                    if (!merged["wget"]) merged["wget"] = "latest";
                    setSelectedTools(merged);
                    // Só marcar como success se não está construindo no momento
                    if (!useStackBuildStore.getState().isBuilding) {
                        setIsSuccess(true);
                    }
                }
            } catch (err) {
                console.error("Failed to load stack config:", err);
            }
        };
        loadConfig();
    }, []);

    const toggleTool = (id: string, defaultVersion?: string) => {
        setIsSuccess(false);
        setSelectedTools((prev) => {
            const next = { ...prev };
            if (next[id]) {
                delete next[id];
            } else {
                next[id] = defaultVersion || "latest";
            }
            return next;
        });
    };

    const changeVersion = (id: string, version: string) => {
        setIsSuccess(false);
        setSelectedTools((prev) => ({
            ...prev,
            [id]: version,
        }));
    };

    const hasCustomTools = useMemo(() => {
        return Object.keys(selectedTools).some((id) => {
            const tool = AVAILABLE_TOOLS.find((t) => t.id === id);
            return tool && tool.category !== "essential";
        });
    }, [selectedTools]);

    const handleBuild = async () => {
        startBuild();
        try {
            // @ts-ignore
            await App.BuildCustomStack(selectedTools);
        } catch (err: any) {
            useStackBuildStore.getState().completeBuild('error', `❌ Falha ao iniciar build: ${err.message}`);
        }
    };

    const getCategoryIcon = (category: string) => {
        switch (category) {
            case "language":
                return <Code2 size={16} />;
            case "tool":
                return <Package size={16} />;
            case "essential":
                return <Server size={16} />;
            default:
                return <Terminal size={16} />;
        }
    };

    return (
        <div className="stack-builder">
            <div className="stack-builder__header">
                <div>
                    <h3 className="stack-builder__title">
                        Construtor de Ambiente
                    </h3>
                    <p className="stack-builder__desc">
                        Selecione as ferramentas e versões para incluir na imagem base dos
                        seus terminais Docker.
                    </p>
                </div>
                <div className="stack-builder__search">
                    <Search size={14} className="search-icon" />
                    <input
                        type="text"
                        placeholder="Filtrar ferramentas..."
                        value={filterText}
                        onChange={(e) => setFilterText(e.target.value)}
                        className="search-input"
                    />
                </div>
            </div>

            <div className="stack-tools-grid">
                {filteredTools.map((tool) => {
                    const isSelected = !!selectedTools[tool.id];
                    const isLocked = tool.isLocked;
                    const currentVersion = selectedTools[tool.id];

                    return (
                        <div
                            key={tool.id}
                            className={`tool-card ${isSelected ? "tool-card--selected" : ""} ${isLocked ? "tool-card--locked" : ""}`}
                            onClick={(e) => {
                                if ((e.target as HTMLElement).tagName === "SELECT") return;
                                if (!isLocked && !isBuilding) {
                                    toggleTool(tool.id, tool.defaultVersion);
                                }
                            }}
                        >
                            <div className="tool-header">
                                <div style={{ opacity: 0.6, display: "flex" }}>
                                    {getCategoryIcon(tool.category)}
                                </div>

                                <div className="tool-icon-wrapper">
                                    {isLocked ? (
                                        <Lock size={14} />
                                    ) : isSelected ? (
                                        <Check size={16} strokeWidth={3} />
                                    ) : (
                                        <Plus size={16} />
                                    )}
                                </div>
                            </div>

                            <div className="tool-info">
                                <span className="tool-name">{tool.name}</span>

                                {isSelected && tool.supportedVersions ? (
                                    <div className="tool-version-select-wrapper">
                                        <select
                                            className="tool-version-select"
                                            value={currentVersion}
                                            onChange={(e) => changeVersion(tool.id, e.target.value)}
                                            onClick={(e) => e.stopPropagation()}
                                        >
                                            {tool.supportedVersions.map(v => (
                                                <option key={v} value={v}>{v}</option>
                                            ))}
                                        </select>
                                    </div>
                                ) : (
                                    <span className="tool-version">
                                        {tool.defaultVersion || ""}
                                    </span>
                                )}
                            </div>
                        </div>
                    );
                })}
            </div>

            <div className="build-console">
                {logs.length === 0 && (
                    <span
                        style={{
                            opacity: 0.5,
                            padding: "1rem",
                            display: "block",
                        }}
                    >
                        O log da construção aparecerá aqui...
                    </span>
                )}
                {logs.map((log, i) => (
                    <p
                        key={i}
                        className={`log-line ${log.includes("❌") ? "log-line--error" : ""} ${log.includes("✅") ? "log-line--success" : ""}`}
                    >
                        {log}
                    </p>
                ))}
                <div ref={logsEndRef} />
            </div>

            <div className="build-actions">
                {isBuilding && (
                    <div className="build-progress">
                        <span className="ascii-loader">{asciiFrames[asciiFrame]}</span>
                        <div className="time-info">
                            <span className="time-elapsed">{formatTime(elapsedTime)}</span>
                            <span className="time-separator">/</span>
                            <span className="time-estimated" title="Tempo estimado baseado na quantidade de ferramentas">
                                Est. {formatTime(estimatedTotalSeconds)}
                            </span>
                        </div>
                        <span className="build-status-text">Construindo imagem...</span>
                    </div>
                )}
                <button
                    className={`btn-build ${hasCustomTools && !isSuccess ? "btn-build--active" : ""}`}
                    onClick={handleBuild}
                    disabled={isBuilding || !hasCustomTools || isSuccess}
                    title={
                        isSuccess
                            ? "Ambiente já atualizado com esta configuração"
                            : !hasCustomTools
                                ? "Selecione pelo menos uma ferramenta para construir o ambiente"
                                : ""
                    }
                >
                    {isBuilding ? "Construindo..." : isSuccess ? "Ambiente Atualizado" : "Construir Ambiente"}
                </button>
            </div>
        </div>
    );
}
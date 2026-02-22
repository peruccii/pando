import type {
    Repository,
    PullRequest,
    Diff,
    Review,
    Comment,
    Issue,
    Branch,
} from "./features/github/types/github";
import type {
    Session,
    GuestRequest,
    ICEServerConfig,
    JoinResult,
} from "./features/session/stores/sessionStore";

/**
 * Declaração centralizada dos bindings Wails (window.go)
 * Todas as funções expostas pelo backend Go ficam aqui.
 */
declare global {
    interface Window {
        go?: {
            main: {
                App: {
                    // === Terminal ===
                    CreateTerminal: (
                        shell: string,
                        cwd: string,
                        useDocker: boolean,
                        cols: number,
                        rows: number,
                    ) => Promise<string>;
                    WriteTerminal: (
                        sessionID: string,
                        data: string,
                    ) => Promise<void>;
                    ResizeTerminal: (
                        sessionID: string,
                        cols: number,
                        rows: number,
                    ) => Promise<void>;
                    DestroyTerminal: (sessionID: string) => Promise<void>;
                    CreateTerminalForAgent: (
                        agentID: number,
                        shell: string,
                        cwd: string,
                        useDocker: boolean,
                        cols: number,
                        rows: number,
                    ) => Promise<string>;
                    CreateTerminalForAgentResume: (
                        agentID: number,
                        cliType: string,
                        shell: string,
                        cwd: string,
                        useDocker: boolean,
                        cols: number,
                        rows: number,
                    ) => Promise<string>;
                    GetTerminals: () => Promise<any[]>;
                    IsTerminalAlive: (sessionID: string) => Promise<boolean>;
                    GetWorkspaceHistoryBuffer: (
                        workspaceID: number,
                    ) => Promise<Record<string, string>>;
                    GetAvailableShells: () => Promise<string[]>;
                    GetAvailableTerminalFonts: () => Promise<string[]>;

                    // === Terminal Snapshots (Session Persistence) ===
                    SaveTerminalSnapshots: (
                        snapshots: TerminalSnapshotDTO[],
                    ) => Promise<void>;
                    GetTerminalSnapshots: () => Promise<TerminalSnapshotDTO[]>;
                    ClearTerminalSnapshots: () => Promise<void>;

                    // === Agents ===
                    CreateAgentSession: (
                        workspaceID: number,
                        name: string,
                        type: string,
                    ) => Promise<AgentSessionDTO>;
                    DeleteAgentSession: (id: number) => Promise<void>;
                    CreateAgent: (name: string, type: string) => Promise<any>;
                    DeleteAgent: (id: number) => Promise<void>;
                    ListAgents: () => Promise<any[]>;
                    SaveAgentLayout: (
                        id: number,
                        layout: string,
                    ) => Promise<void>;

                    // === Workspaces ===
                    GetWorkspacesWithAgents: () => Promise<WorkspaceWithAgentsDTO[]>;
                    CreateWorkspace: (name: string) => Promise<WorkspaceWithAgentsDTO>;
                    SyncGuestWorkspace: (name: string) => Promise<WorkspaceWithAgentsDTO>;
                    RenameWorkspace: (id: number, name: string) => Promise<WorkspaceWithAgentsDTO>;
                    SetWorkspaceColor: (id: number, color: string) => Promise<WorkspaceWithAgentsDTO>;
                    DeleteWorkspace: (id: number) => Promise<void>;
                    SetActiveWorkspace: (id: number) => Promise<void>;

                    // === Layout ===
                    SaveLayoutState: (json: string) => Promise<void>;
                    GetLayoutState: () => Promise<string>;
                    GetHydrationData: () => Promise<any>;
                    SaveTheme: (theme: string) => Promise<void>;
                    SaveLanguage: (language: string) => Promise<void>;
                    SaveDefaultShell: (shell: string) => Promise<void>;
                    SaveTerminalFontSize: (size: number) => Promise<void>;
                    SaveTerminalFontFamily: (family: string) => Promise<void>;
                    SaveShortcutBindings: (bindingsJSON: string) => Promise<void>;
                    CompleteOnboarding: () => Promise<void>;

                    // === GitHub ===
                    GHListRepositories: () => Promise<Repository[]>;
                    GHListPullRequests: (
                        owner: string,
                        repo: string,
                        state: string,
                        first: number,
                    ) => Promise<PullRequest[]>;
                    GHGetPullRequest: (
                        owner: string,
                        repo: string,
                        number: number,
                    ) => Promise<PullRequest>;
                    GHGetPullRequestDiff: (
                        owner: string,
                        repo: string,
                        number: number,
                        first: number,
                        after: string,
                    ) => Promise<Diff>;
                    GHCreatePullRequest: (
                        owner: string,
                        repo: string,
                        title: string,
                        body: string,
                        head: string,
                        base: string,
                        isDraft: boolean,
                    ) => Promise<PullRequest>;
                    GHMergePullRequest: (
                        owner: string,
                        repo: string,
                        number: number,
                        method: string,
                    ) => Promise<void>;
                    GHClosePullRequest: (
                        owner: string,
                        repo: string,
                        number: number,
                    ) => Promise<void>;
                    GHListReviews: (
                        owner: string,
                        repo: string,
                        prNumber: number,
                    ) => Promise<Review[]>;
                    GHCreateReview: (
                        owner: string,
                        repo: string,
                        prNumber: number,
                        body: string,
                        event: string,
                    ) => Promise<Review>;
                    GHListComments: (
                        owner: string,
                        repo: string,
                        prNumber: number,
                    ) => Promise<Comment[]>;
                    GHCreateComment: (
                        owner: string,
                        repo: string,
                        prNumber: number,
                        body: string,
                    ) => Promise<Comment>;
                    GHCreateInlineComment: (
                        owner: string,
                        repo: string,
                        prNumber: number,
                        body: string,
                        path: string,
                        line: number,
                        side: string,
                    ) => Promise<Comment>;
                    GHListIssues: (
                        owner: string,
                        repo: string,
                        state: string,
                        first: number,
                    ) => Promise<Issue[]>;
                    GHCreateIssue: (
                        owner: string,
                        repo: string,
                        title: string,
                        body: string,
                    ) => Promise<Issue>;
                    GHUpdateIssue: (
                        owner: string,
                        repo: string,
                        number: number,
                        title: string | null,
                        body: string | null,
                        state: string | null,
                    ) => Promise<void>;
                    GHListBranches: (
                        owner: string,
                        repo: string,
                    ) => Promise<Branch[]>;
                    GHCreateBranch: (
                        owner: string,
                        repo: string,
                        name: string,
                        sourceBranch: string,
                    ) => Promise<Branch>;
                    GHInvalidateCache: (
                        owner: string,
                        repo: string,
                    ) => Promise<void>;

                    // === FileWatcher ===
                    WatchProject: (projectPath: string) => Promise<void>;
                    UnwatchProject: (projectPath: string) => Promise<void>;
                    GetCurrentBranch: (projectPath: string) => Promise<string>;
                    GetLastCommit: (projectPath: string) => Promise<{
                        hash: string;
                        message: string;
                        author: string;
                        date: string;
                    }>;

                    // === Polling ===
                    StartPolling: (
                        owner: string,
                        repo: string,
                    ) => Promise<void>;
                    StopPolling: () => Promise<void>;
                    SetPollingContext: (context: string) => Promise<void>;
                    GetRateLimitInfo: () => Promise<{
                        remaining: number;
                        limit: number;
                        resetAt: string;
                    }>;

                    // === Session / P2P ===
                    SessionCreate: (
                        maxGuests: number,
                        mode: string,
                        allowAnonymous: boolean,
                        workspaceID: number,
                    ) => Promise<Session>;
                    SessionJoin: (
                        code: string,
                        name: string,
                        email: string,
                    ) => Promise<JoinResult>;
                    SessionApproveGuest: (
                        sessionID: string,
                        guestUserID: string,
                    ) => Promise<void>;
                    SessionRejectGuest: (
                        sessionID: string,
                        guestUserID: string,
                    ) => Promise<void>;
                    SessionEnd: (sessionID: string) => Promise<void>;
                    SessionListPendingGuests: (
                        sessionID: string,
                    ) => Promise<GuestRequest[]>;
                    SessionSetGuestPermission: (
                        sessionID: string,
                        guestUserID: string,
                        permission: string,
                    ) => Promise<void>;
                    SessionKickGuest: (
                        sessionID: string,
                        guestUserID: string,
                    ) => Promise<void>;
                    SessionRegenerateCode: (
                        sessionID: string,
                    ) => Promise<Session>;
                    SessionRevokeCode: (
                        sessionID: string,
                    ) => Promise<Session>;
                    SessionSetAllowNewJoins: (
                        sessionID: string,
                        allow: boolean,
                    ) => Promise<Session>;
                    SessionGetActive: () => Promise<Session | null>;
                    SessionGetSession: (
                        sessionID: string,
                    ) => Promise<Session | null>;
                    SessionGetICEServers: () => Promise<ICEServerConfig[]>;
                    SessionGetAuditLogs: (
                        sessionID: string,
                        limit: number,
                    ) => Promise<
                        Array<{
                            id: number;
                            sessionID: string;
                            userID: string;
                            action: string;
                            details: string;
                            createdAt: string;
                        }>
                    >;
                    SessionRestartEnvironment: (
                        sessionID: string,
                    ) => Promise<void>;
                    DockerIsAvailable: () => Promise<boolean>;
                };
            };
        };
    }
    /** DTO para persistência de sessões de terminal/CLI */
    interface TerminalSnapshotDTO {
        paneId: string;
        sessionId?: string;
        paneTitle: string;
        paneType: string;
        shell: string;
        cwd: string;
        useDocker: boolean;
        config?: string;
        cliType?: string;
    }
    interface AgentSessionDTO {
        id: number;
        workspaceId: number;
        name: string;
        type: string;
        shell: string;
        cwd: string;
        useDocker: boolean;
        sessionId?: string;
        status: string;
        sortOrder: number;
        isMinimized: boolean;
        createdAt?: string;
        updatedAt?: string;
    }
    interface WorkspaceWithAgentsDTO {
        id: number;
        userId: string;
        name: string;
        path: string;
        gitRemote?: string;
        owner?: string;
        repo?: string;
        color?: string;
        isActive: boolean;
        lastOpenedAt?: string;
        createdAt?: string;
        updatedAt?: string;
        agents: AgentSessionDTO[];
    }
}

export { };

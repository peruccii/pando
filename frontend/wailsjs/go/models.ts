export namespace ai {
	
	export class AIProvider {
	    id: string;
	    name: string;
	    model: string;
	    apiKey?: string;
	    endpoint?: string;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AIProvider(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.model = source["model"];
	        this.apiKey = source["apiKey"];
	        this.endpoint = source["endpoint"];
	        this.enabled = source["enabled"];
	    }
	}
	export class IssueContext {
	    owner?: string;
	    repo?: string;
	    number: number;
	    title?: string;
	    body?: string;
	    state?: string;
	
	    static createFrom(source: any = {}) {
	        return new IssueContext(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.owner = source["owner"];
	        this.repo = source["repo"];
	        this.number = source["number"];
	        this.title = source["title"];
	        this.body = source["body"];
	        this.state = source["state"];
	    }
	}
	export class PRContext {
	    owner?: string;
	    repo?: string;
	    number: number;
	    title?: string;
	    body?: string;
	    diff?: string;
	    author?: string;
	    branch?: string;
	
	    static createFrom(source: any = {}) {
	        return new PRContext(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.owner = source["owner"];
	        this.repo = source["repo"];
	        this.number = source["number"];
	        this.title = source["title"];
	        this.body = source["body"];
	        this.diff = source["diff"];
	        this.author = source["author"];
	        this.branch = source["branch"];
	    }
	}
	export class SessionState {
	    projectName: string;
	    projectPath?: string;
	    currentBranch: string;
	    currentFile?: string;
	    activePR?: PRContext;
	    activeIssue?: IssueContext;
	    lastCommand?: string;
	    lastStdout?: string;
	    lastStderr?: string;
	    shellType?: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projectName = source["projectName"];
	        this.projectPath = source["projectPath"];
	        this.currentBranch = source["currentBranch"];
	        this.currentFile = source["currentFile"];
	        this.activePR = this.convertValues(source["activePR"], PRContext);
	        this.activeIssue = this.convertValues(source["activeIssue"], IssueContext);
	        this.lastCommand = source["lastCommand"];
	        this.lastStdout = source["lastStdout"];
	        this.lastStderr = source["lastStderr"];
	        this.shellType = source["shellType"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace auth {
	
	export class User {
	    id: string;
	    email: string;
	    name: string;
	    avatarUrl?: string;
	    provider: string;
	
	    static createFrom(source: any = {}) {
	        return new User(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.email = source["email"];
	        this.name = source["name"];
	        this.avatarUrl = source["avatarUrl"];
	        this.provider = source["provider"];
	    }
	}
	export class AuthState {
	    isAuthenticated: boolean;
	    user?: User;
	    provider?: string;
	    hasGitHubToken: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AuthState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.isAuthenticated = source["isAuthenticated"];
	        this.user = this.convertValues(source["user"], User);
	        this.provider = source["provider"];
	        this.hasGitHubToken = source["hasGitHubToken"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace database {
	
	export class AgentSession {
	    id: number;
	    workspaceId: number;
	    name: string;
	    type: string;
	    shell: string;
	    cwd: string;
	    useDocker: boolean;
	    sessionId?: string;
	    status: string;
	    layoutJson?: string;
	    sortOrder: number;
	    isMinimized: boolean;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new AgentSession(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.workspaceId = source["workspaceId"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.shell = source["shell"];
	        this.cwd = source["cwd"];
	        this.useDocker = source["useDocker"];
	        this.sessionId = source["sessionId"];
	        this.status = source["status"];
	        this.layoutJson = source["layoutJson"];
	        this.sortOrder = source["sortOrder"];
	        this.isMinimized = source["isMinimized"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AuditLog {
	    id: number;
	    sessionID: string;
	    userID: string;
	    action: string;
	    details: string;
	    // Go type: time
	    createdAt: any;
	
	    static createFrom(source: any = {}) {
	        return new AuditLog(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.sessionID = source["sessionID"];
	        this.userID = source["userID"];
	        this.action = source["action"];
	        this.details = source["details"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Workspace {
	    id: number;
	    userId: string;
	    name: string;
	    path: string;
	    gitRemote?: string;
	    owner?: string;
	    repo?: string;
	    color?: string;
	    isActive: boolean;
	    agents?: AgentSession[];
	    // Go type: time
	    lastOpenedAt?: any;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Workspace(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.userId = source["userId"];
	        this.name = source["name"];
	        this.path = source["path"];
	        this.gitRemote = source["gitRemote"];
	        this.owner = source["owner"];
	        this.repo = source["repo"];
	        this.color = source["color"];
	        this.isActive = source["isActive"];
	        this.agents = this.convertValues(source["agents"], AgentSession);
	        this.lastOpenedAt = this.convertValues(source["lastOpenedAt"], null);
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace filewatcher {
	
	export class CommitInfo {
	    hash: string;
	    message: string;
	    author: string;
	    // Go type: time
	    date: any;
	
	    static createFrom(source: any = {}) {
	        return new CommitInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hash = source["hash"];
	        this.message = source["message"];
	        this.author = source["author"];
	        this.date = this.convertValues(source["date"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace gitactivity {
	
	export class EventFile {
	    path: string;
	    status?: string;
	    added?: number;
	    removed?: number;
	
	    static createFrom(source: any = {}) {
	        return new EventFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.status = source["status"];
	        this.added = source["added"];
	        this.removed = source["removed"];
	    }
	}
	export class EventDetails {
	    ref?: string;
	    commitHash?: string;
	    diffPreview?: string;
	    files?: EventFile[];
	    extra?: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new EventDetails(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = source["ref"];
	        this.commitHash = source["commitHash"];
	        this.diffPreview = source["diffPreview"];
	        this.files = this.convertValues(source["files"], EventFile);
	        this.extra = source["extra"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Event {
	    id: string;
	    type: string;
	    actorName: string;
	    actorId?: string;
	    repoPath: string;
	    repoName: string;
	    branch?: string;
	    message: string;
	    // Go type: time
	    timestamp: any;
	    source?: string;
	    dedupeKey?: string;
	    details?: EventDetails;
	
	    static createFrom(source: any = {}) {
	        return new Event(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.type = source["type"];
	        this.actorName = source["actorName"];
	        this.actorId = source["actorId"];
	        this.repoPath = source["repoPath"];
	        this.repoName = source["repoName"];
	        this.branch = source["branch"];
	        this.message = source["message"];
	        this.timestamp = this.convertValues(source["timestamp"], null);
	        this.source = source["source"];
	        this.dedupeKey = source["dedupeKey"];
	        this.details = this.convertValues(source["details"], EventDetails);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	

}

export namespace github {
	
	export class Branch {
	    name: string;
	    prefix: string;
	    commit: string;
	
	    static createFrom(source: any = {}) {
	        return new Branch(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.prefix = source["prefix"];
	        this.commit = source["commit"];
	    }
	}
	export class User {
	    login: string;
	    avatarUrl: string;
	
	    static createFrom(source: any = {}) {
	        return new User(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.login = source["login"];
	        this.avatarUrl = source["avatarUrl"];
	    }
	}
	export class Comment {
	    id: string;
	    author: User;
	    body: string;
	    path?: string;
	    line?: number;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Comment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.author = this.convertValues(source["author"], User);
	        this.body = source["body"];
	        this.path = source["path"];
	        this.line = source["line"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DiffPagination {
	    first: number;
	    after?: string;
	    hasNextPage: boolean;
	    endCursor?: string;
	
	    static createFrom(source: any = {}) {
	        return new DiffPagination(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.first = source["first"];
	        this.after = source["after"];
	        this.hasNextPage = source["hasNextPage"];
	        this.endCursor = source["endCursor"];
	    }
	}
	export class DiffLine {
	    type: string;
	    content: string;
	    oldLine?: number;
	    newLine?: number;
	
	    static createFrom(source: any = {}) {
	        return new DiffLine(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.content = source["content"];
	        this.oldLine = source["oldLine"];
	        this.newLine = source["newLine"];
	    }
	}
	export class DiffHunk {
	    oldStart: number;
	    oldLines: number;
	    newStart: number;
	    newLines: number;
	    header: string;
	    lines: DiffLine[];
	
	    static createFrom(source: any = {}) {
	        return new DiffHunk(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.oldStart = source["oldStart"];
	        this.oldLines = source["oldLines"];
	        this.newStart = source["newStart"];
	        this.newLines = source["newLines"];
	        this.header = source["header"];
	        this.lines = this.convertValues(source["lines"], DiffLine);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DiffFile {
	    filename: string;
	    status: string;
	    additions: number;
	    deletions: number;
	    patch?: string;
	    hunks: DiffHunk[];
	
	    static createFrom(source: any = {}) {
	        return new DiffFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.filename = source["filename"];
	        this.status = source["status"];
	        this.additions = source["additions"];
	        this.deletions = source["deletions"];
	        this.patch = source["patch"];
	        this.hunks = this.convertValues(source["hunks"], DiffHunk);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Diff {
	    files: DiffFile[];
	    totalFiles: number;
	    pagination: DiffPagination;
	
	    static createFrom(source: any = {}) {
	        return new Diff(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.files = this.convertValues(source["files"], DiffFile);
	        this.totalFiles = source["totalFiles"];
	        this.pagination = this.convertValues(source["pagination"], DiffPagination);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	
	export class Label {
	    name: string;
	    color: string;
	
	    static createFrom(source: any = {}) {
	        return new Label(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.color = source["color"];
	    }
	}
	export class Issue {
	    id: string;
	    number: number;
	    title: string;
	    body: string;
	    state: string;
	    author: User;
	    assignees: User[];
	    labels: Label[];
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Issue(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.number = source["number"];
	        this.title = source["title"];
	        this.body = source["body"];
	        this.state = source["state"];
	        this.author = this.convertValues(source["author"], User);
	        this.assignees = this.convertValues(source["assignees"], User);
	        this.labels = this.convertValues(source["labels"], Label);
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class PullRequest {
	    id: string;
	    number: number;
	    title: string;
	    body: string;
	    state: string;
	    author: User;
	    reviewers: User[];
	    labels: Label[];
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	    mergeCommit?: string;
	    headBranch: string;
	    baseBranch: string;
	    additions: number;
	    deletions: number;
	    changedFiles: number;
	    isDraft: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PullRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.number = source["number"];
	        this.title = source["title"];
	        this.body = source["body"];
	        this.state = source["state"];
	        this.author = this.convertValues(source["author"], User);
	        this.reviewers = this.convertValues(source["reviewers"], User);
	        this.labels = this.convertValues(source["labels"], Label);
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	        this.mergeCommit = source["mergeCommit"];
	        this.headBranch = source["headBranch"];
	        this.baseBranch = source["baseBranch"];
	        this.additions = source["additions"];
	        this.deletions = source["deletions"];
	        this.changedFiles = source["changedFiles"];
	        this.isDraft = source["isDraft"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RateLimitInfo {
	    remaining: number;
	    limit: number;
	    // Go type: time
	    resetAt: any;
	
	    static createFrom(source: any = {}) {
	        return new RateLimitInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.remaining = source["remaining"];
	        this.limit = source["limit"];
	        this.resetAt = this.convertValues(source["resetAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Repository {
	    id: string;
	    name: string;
	    fullName: string;
	    owner: string;
	    description: string;
	    isPrivate: boolean;
	    defaultBranch: string;
	    // Go type: time
	    updatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Repository(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.fullName = source["fullName"];
	        this.owner = source["owner"];
	        this.description = source["description"];
	        this.isPrivate = source["isPrivate"];
	        this.defaultBranch = source["defaultBranch"];
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Review {
	    id: string;
	    author: User;
	    state: string;
	    body: string;
	    // Go type: time
	    createdAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Review(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.author = this.convertValues(source["author"], User);
	        this.state = source["state"];
	        this.body = source["body"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace main {
	
	export class HydrationPayload {
	    isAuthenticated: boolean;
	    user?: auth.User;
	    theme: string;
	    language: string;
	    defaultShell: string;
	    terminalFontSize: number;
	    onboardingCompleted: boolean;
	    shortcutBindings?: string;
	    version: string;
	    workspaces?: database.Workspace[];
	
	    static createFrom(source: any = {}) {
	        return new HydrationPayload(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.isAuthenticated = source["isAuthenticated"];
	        this.user = this.convertValues(source["user"], auth.User);
	        this.theme = source["theme"];
	        this.language = source["language"];
	        this.defaultShell = source["defaultShell"];
	        this.terminalFontSize = source["terminalFontSize"];
	        this.onboardingCompleted = source["onboardingCompleted"];
	        this.shortcutBindings = source["shortcutBindings"];
	        this.version = source["version"];
	        this.workspaces = this.convertValues(source["workspaces"], database.Workspace);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class StackBuildState {
	    isBuilding: boolean;
	    logs: string[];
	    startTime: number;
	    result: string;
	
	    static createFrom(source: any = {}) {
	        return new StackBuildState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.isBuilding = source["isBuilding"];
	        this.logs = source["logs"];
	        this.startTime = source["startTime"];
	        this.result = source["result"];
	    }
	}
	export class TerminalSnapshotDTO {
	    paneId: string;
	    sessionId: string;
	    paneTitle: string;
	    paneType: string;
	    shell: string;
	    cwd: string;
	    useDocker: boolean;
	    config?: string;
	    cliType?: string;
	
	    static createFrom(source: any = {}) {
	        return new TerminalSnapshotDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.paneId = source["paneId"];
	        this.sessionId = source["sessionId"];
	        this.paneTitle = source["paneTitle"];
	        this.paneType = source["paneType"];
	        this.shell = source["shell"];
	        this.cwd = source["cwd"];
	        this.useDocker = source["useDocker"];
	        this.config = source["config"];
	        this.cliType = source["cliType"];
	    }
	}

}

export namespace session {
	
	export class GuestRequest {
	    userID: string;
	    name: string;
	    email?: string;
	    avatarUrl?: string;
	    // Go type: time
	    requestAt: any;
	
	    static createFrom(source: any = {}) {
	        return new GuestRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.userID = source["userID"];
	        this.name = source["name"];
	        this.email = source["email"];
	        this.avatarUrl = source["avatarUrl"];
	        this.requestAt = this.convertValues(source["requestAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ICEServerConfig {
	    urls: string[];
	    username?: string;
	    credential?: string;
	
	    static createFrom(source: any = {}) {
	        return new ICEServerConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.urls = source["urls"];
	        this.username = source["username"];
	        this.credential = source["credential"];
	    }
	}
	export class JoinResult {
	    sessionID: string;
	    hostName: string;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new JoinResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionID = source["sessionID"];
	        this.hostName = source["hostName"];
	        this.status = source["status"];
	    }
	}
	export class SessionConfig {
	    maxGuests: number;
	    defaultPerm: string;
	    allowAnonymous: boolean;
	    mode: string;
	    dockerImage?: string;
	    projectPath?: string;
	    codeTTLMinutes: number;
	
	    static createFrom(source: any = {}) {
	        return new SessionConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.maxGuests = source["maxGuests"];
	        this.defaultPerm = source["defaultPerm"];
	        this.allowAnonymous = source["allowAnonymous"];
	        this.mode = source["mode"];
	        this.dockerImage = source["dockerImage"];
	        this.projectPath = source["projectPath"];
	        this.codeTTLMinutes = source["codeTTLMinutes"];
	    }
	}
	export class SessionGuest {
	    userID: string;
	    name: string;
	    avatarUrl?: string;
	    permission: string;
	    // Go type: time
	    joinedAt: any;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionGuest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.userID = source["userID"];
	        this.name = source["name"];
	        this.avatarUrl = source["avatarUrl"];
	        this.permission = source["permission"];
	        this.joinedAt = this.convertValues(source["joinedAt"], null);
	        this.status = source["status"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Session {
	    id: string;
	    code: string;
	    hostUserID: string;
	    hostName: string;
	    status: string;
	    mode: string;
	    guests: SessionGuest[];
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    expiresAt: any;
	    config: SessionConfig;
	
	    static createFrom(source: any = {}) {
	        return new Session(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.code = source["code"];
	        this.hostUserID = source["hostUserID"];
	        this.hostName = source["hostName"];
	        this.status = source["status"];
	        this.mode = source["mode"];
	        this.guests = this.convertValues(source["guests"], SessionGuest);
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.expiresAt = this.convertValues(source["expiresAt"], null);
	        this.config = this.convertValues(source["config"], SessionConfig);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	

}

export namespace terminal {
	
	export class SessionInfo {
	    id: string;
	    shell: string;
	    cwd: string;
	    cols: number;
	    rows: number;
	    isAlive: boolean;
	    // Go type: time
	    createdAt: any;
	
	    static createFrom(source: any = {}) {
	        return new SessionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.shell = source["shell"];
	        this.cwd = source["cwd"];
	        this.cols = source["cols"];
	        this.rows = source["rows"];
	        this.isAlive = source["isAlive"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}


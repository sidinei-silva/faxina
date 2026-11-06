export namespace clean {
	
	export class Entry {
	    itemId: string;
	    name: string;
	    status: string;
	    command: string;
	    sizeBytes: number;
	    sizeEstimated: boolean;
	    needsRoot: boolean;
	    refused: string;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.itemId = source["itemId"];
	        this.name = source["name"];
	        this.status = source["status"];
	        this.command = source["command"];
	        this.sizeBytes = source["sizeBytes"];
	        this.sizeEstimated = source["sizeEstimated"];
	        this.needsRoot = source["needsRoot"];
	        this.refused = source["refused"];
	    }
	}
	export class Outcome {
	    itemId: string;
	    name: string;
	    command: string;
	    ok: boolean;
	    skipped: boolean;
	    error: string;
	    sizeBytes: number;
	
	    static createFrom(source: any = {}) {
	        return new Outcome(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.itemId = source["itemId"];
	        this.name = source["name"];
	        this.command = source["command"];
	        this.ok = source["ok"];
	        this.skipped = source["skipped"];
	        this.error = source["error"];
	        this.sizeBytes = source["sizeBytes"];
	    }
	}
	export class Plan {
	    id: string;
	    entries: Entry[];
	    totalBytes: number;
	    accepted: number;
	    refused: number;
	    needsRoot: boolean;
	    hasCandidate: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Plan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.entries = this.convertValues(source["entries"], Entry);
	        this.totalBytes = source["totalBytes"];
	        this.accepted = source["accepted"];
	        this.refused = source["refused"];
	        this.needsRoot = source["needsRoot"];
	        this.hasCandidate = source["hasCandidate"];
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
	export class Report {
	    planId: string;
	    dryRun: boolean;
	    canceled: boolean;
	    outcomes: Outcome[];
	    estimatedBytes: number;
	    freedBytes: number;
	    before: model.Disk;
	    after: model.Disk;
	    note: string;
	    auditPath: string;
	    auditError: string;
	
	    static createFrom(source: any = {}) {
	        return new Report(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.planId = source["planId"];
	        this.dryRun = source["dryRun"];
	        this.canceled = source["canceled"];
	        this.outcomes = this.convertValues(source["outcomes"], Outcome);
	        this.estimatedBytes = source["estimatedBytes"];
	        this.freedBytes = source["freedBytes"];
	        this.before = this.convertValues(source["before"], model.Disk);
	        this.after = this.convertValues(source["after"], model.Disk);
	        this.note = source["note"];
	        this.auditPath = source["auditPath"];
	        this.auditError = source["auditError"];
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

export namespace model {
	
	export class Action {
	    kind: string;
	    target: string;
	    command: string;
	    needsRoot: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Action(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.target = source["target"];
	        this.command = source["command"];
	        this.needsRoot = source["needsRoot"];
	    }
	}
	export class Disk {
	    mount: string;
	    totalBytes: number;
	    freeBytes: number;
	    usedBytes: number;
	    usedPercent: number;
	
	    static createFrom(source: any = {}) {
	        return new Disk(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mount = source["mount"];
	        this.totalBytes = source["totalBytes"];
	        this.freeBytes = source["freeBytes"];
	        this.usedBytes = source["usedBytes"];
	        this.usedPercent = source["usedPercent"];
	    }
	}
	export class Facts {
	    Containers: number;
	    Anonymous: boolean;
	    Content: string;
	    // Go type: time
	    CreatedAt: any;
	    // Go type: time
	    LastActivity: any;
	    Repo: string;
	    Tag: string;
	    ImageID: string;
	    NewestInRepo: boolean;
	    OlderDuplicate: boolean;
	    LocalRegistry: boolean;
	    Infra: boolean;
	    Deployed: boolean;
	    KubeUnknown: boolean;
	    OpenByProcess: boolean;
	    HasLockfile: boolean;
	    ProjectActive: boolean;
	    Label: string;
	    ManualHint: string;
	
	    static createFrom(source: any = {}) {
	        return new Facts(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Containers = source["Containers"];
	        this.Anonymous = source["Anonymous"];
	        this.Content = source["Content"];
	        this.CreatedAt = this.convertValues(source["CreatedAt"], null);
	        this.LastActivity = this.convertValues(source["LastActivity"], null);
	        this.Repo = source["Repo"];
	        this.Tag = source["Tag"];
	        this.ImageID = source["ImageID"];
	        this.NewestInRepo = source["NewestInRepo"];
	        this.OlderDuplicate = source["OlderDuplicate"];
	        this.LocalRegistry = source["LocalRegistry"];
	        this.Infra = source["Infra"];
	        this.Deployed = source["Deployed"];
	        this.KubeUnknown = source["KubeUnknown"];
	        this.OpenByProcess = source["OpenByProcess"];
	        this.HasLockfile = source["HasLockfile"];
	        this.ProjectActive = source["ProjectActive"];
	        this.Label = source["Label"];
	        this.ManualHint = source["ManualHint"];
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
	export class Item {
	    id: string;
	    source: string;
	    group: string;
	    name: string;
	    detail: string;
	    path: string;
	    sizeBytes: number;
	    sizeEstimated: boolean;
	    ageDays: number;
	    status: string;
	    reason: string;
	    manualHint: string;
	    action: Action;
	
	    static createFrom(source: any = {}) {
	        return new Item(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.source = source["source"];
	        this.group = source["group"];
	        this.name = source["name"];
	        this.detail = source["detail"];
	        this.path = source["path"];
	        this.sizeBytes = source["sizeBytes"];
	        this.sizeEstimated = source["sizeEstimated"];
	        this.ageDays = source["ageDays"];
	        this.status = source["status"];
	        this.reason = source["reason"];
	        this.manualHint = source["manualHint"];
	        this.action = this.convertValues(source["action"], Action);
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
	export class ScanResult {
	    // Go type: time
	    scannedAt: any;
	    durationMs: number;
	    disk: Disk;
	    items: Item[];
	    warnings: string[];
	
	    static createFrom(source: any = {}) {
	        return new ScanResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scannedAt = this.convertValues(source["scannedAt"], null);
	        this.durationMs = source["durationMs"];
	        this.disk = this.convertValues(source["disk"], Disk);
	        this.items = this.convertValues(source["items"], Item);
	        this.warnings = source["warnings"];
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


export namespace wails {
	
	export class FileItem {
	    storage_id: string;
	    path: string;
	    name: string;
	    size: number;
	    modified_at: string;
	    is_symlink: boolean;
	    quick_hash?: string;
	    content_sha256?: string;
	    physical_device?: number;
	    physical_inode?: number;
	    physical_link_count?: number;
	    physical_reliable: boolean;
	    format_kind?: string;
	    format_mime?: string;
	
	    static createFrom(source: any = {}) {
	        return new FileItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.storage_id = source["storage_id"];
	        this.path = source["path"];
	        this.name = source["name"];
	        this.size = source["size"];
	        this.modified_at = source["modified_at"];
	        this.is_symlink = source["is_symlink"];
	        this.quick_hash = source["quick_hash"];
	        this.content_sha256 = source["content_sha256"];
	        this.physical_device = source["physical_device"];
	        this.physical_inode = source["physical_inode"];
	        this.physical_link_count = source["physical_link_count"];
	        this.physical_reliable = source["physical_reliable"];
	        this.format_kind = source["format_kind"];
	        this.format_mime = source["format_mime"];
	    }
	}
	export class GroupDetailResponse {
	    group_id: string;
	    sha256: string;
	    size: number;
	    storage_id: string;
	    path_count: number;
	    physical_copy_count: number;
	    hardlink_alias_count: number;
	    physical_reclaimable_bytes: number;
	    sample_path: string;
	    decision_type?: string;
	    files: FileItem[];
	
	    static createFrom(source: any = {}) {
	        return new GroupDetailResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.group_id = source["group_id"];
	        this.sha256 = source["sha256"];
	        this.size = source["size"];
	        this.storage_id = source["storage_id"];
	        this.path_count = source["path_count"];
	        this.physical_copy_count = source["physical_copy_count"];
	        this.hardlink_alias_count = source["hardlink_alias_count"];
	        this.physical_reclaimable_bytes = source["physical_reclaimable_bytes"];
	        this.sample_path = source["sample_path"];
	        this.decision_type = source["decision_type"];
	        this.files = this.convertValues(source["files"], FileItem);
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
	export class GroupSummary {
	    group_id: string;
	    sha256: string;
	    size: number;
	    storage_id: string;
	    path_count: number;
	    physical_copy_count: number;
	    hardlink_alias_count: number;
	    physical_reclaimable_bytes: number;
	    sample_path: string;
	    decision_type?: string;
	
	    static createFrom(source: any = {}) {
	        return new GroupSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.group_id = source["group_id"];
	        this.sha256 = source["sha256"];
	        this.size = source["size"];
	        this.storage_id = source["storage_id"];
	        this.path_count = source["path_count"];
	        this.physical_copy_count = source["physical_copy_count"];
	        this.hardlink_alias_count = source["hardlink_alias_count"];
	        this.physical_reclaimable_bytes = source["physical_reclaimable_bytes"];
	        this.sample_path = source["sample_path"];
	        this.decision_type = source["decision_type"];
	    }
	}
	export class JobEvent {
	    sequence: number;
	    event_type: string;
	    stage: string;
	    state: string;
	    payload?: Record<string, any>;
	    created_at: string;
	
	    static createFrom(source: any = {}) {
	        return new JobEvent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sequence = source["sequence"];
	        this.event_type = source["event_type"];
	        this.stage = source["stage"];
	        this.state = source["state"];
	        this.payload = source["payload"];
	        this.created_at = source["created_at"];
	    }
	}
	export class JobDetailResponse {
	    job_id: string;
	    state: string;
	    stage: string;
	    discovered: number;
	    processed: number;
	    failed: number;
	    warning_count: number;
	    error_code?: string;
	    created_at: string;
	    started_at?: string;
	    completed_at?: string;
	    events: JobEvent[];
	
	    static createFrom(source: any = {}) {
	        return new JobDetailResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.job_id = source["job_id"];
	        this.state = source["state"];
	        this.stage = source["stage"];
	        this.discovered = source["discovered"];
	        this.processed = source["processed"];
	        this.failed = source["failed"];
	        this.warning_count = source["warning_count"];
	        this.error_code = source["error_code"];
	        this.created_at = source["created_at"];
	        this.started_at = source["started_at"];
	        this.completed_at = source["completed_at"];
	        this.events = this.convertValues(source["events"], JobEvent);
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
	
	export class JobSummary {
	    job_id: string;
	    job_type: string;
	    state: string;
	    stage: string;
	    discovered?: number;
	    processed?: number;
	    failed?: number;
	    error_code?: string;
	    created_at: string;
	    completed_at?: string;

	    static createFrom(source: any = {}) {
	        return new JobSummary(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.job_id = source["job_id"];
	        this.job_type = source["job_type"];
	        this.state = source["state"];
	        this.stage = source["stage"];
	        this.discovered = source["discovered"];
	        this.processed = source["processed"];
	        this.failed = source["failed"];
	        this.error_code = source["error_code"];
	        this.created_at = source["created_at"];
	        this.completed_at = source["completed_at"];
	    }
	}
	export class ListGroupsRequest {
	    storage_id?: string;
	    page_size?: number;
	    cursor?: string;
	    min_reclaimable_bytes?: number;
	
	    static createFrom(source: any = {}) {
	        return new ListGroupsRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.storage_id = source["storage_id"];
	        this.page_size = source["page_size"];
	        this.cursor = source["cursor"];
	        this.min_reclaimable_bytes = source["min_reclaimable_bytes"];
	    }
	}
	export class ListGroupsResponse {
	    groups: GroupSummary[];
	    next_cursor?: string;
	    total_count: number;
	
	    static createFrom(source: any = {}) {
	        return new ListGroupsResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.groups = this.convertValues(source["groups"], GroupSummary);
	        this.next_cursor = source["next_cursor"];
	        this.total_count = source["total_count"];
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
	export class ProjectInfo {
	    path: string;
	    is_open: boolean;
	    storage_count: number;
	
	    static createFrom(source: any = {}) {
	        return new ProjectInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.is_open = source["is_open"];
	        this.storage_count = source["storage_count"];
	    }
	}
	export class ScanJobProgress {
	    job_id: string;
	    state: string;
	    stage: string;
	    discovered: number;
	    processed: number;
	    failed: number;
	    warning_count: number;
	    error_code?: string;
	    created_at: string;
	    started_at?: string;
	    completed_at?: string;
	
	    static createFrom(source: any = {}) {
	        return new ScanJobProgress(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.job_id = source["job_id"];
	        this.state = source["state"];
	        this.stage = source["stage"];
	        this.discovered = source["discovered"];
	        this.processed = source["processed"];
	        this.failed = source["failed"];
	        this.warning_count = source["warning_count"];
	        this.error_code = source["error_code"];
	        this.created_at = source["created_at"];
	        this.started_at = source["started_at"];
	        this.completed_at = source["completed_at"];
	    }
	}
	export class StartScanRequest {
	    root: string;
	    storage_id: string;
	    full_scan?: boolean;
	    workers?: number;
	
	    static createFrom(source: any = {}) {
	        return new StartScanRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.root = source["root"];
	        this.storage_id = source["storage_id"];
	        this.full_scan = source["full_scan"];
	        this.workers = source["workers"];
	    }
	}
	export class StartScanResponse {
	    job_id: string;
	
	    static createFrom(source: any = {}) {
	        return new StartScanResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.job_id = source["job_id"];
	    }
	}
	export class StorageInfo {
	    id: string;
	    root_path: string;
	    kind: string;
	    created_at: string;
	
	    static createFrom(source: any = {}) {
	        return new StorageInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.root_path = source["root_path"];
	        this.kind = source["kind"];
	        this.created_at = source["created_at"];
	    }
	}
	export class VersionInfo {
	    version: string;
	    commit: string;
	    build_time: string;
	
	    static createFrom(source: any = {}) {
	        return new VersionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.commit = source["commit"];
	        this.build_time = source["build_time"];
	    }
	}

}


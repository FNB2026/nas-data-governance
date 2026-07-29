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

	// ---- V5 Diagnostic Types ----

	export class DiagnoseFormatsRequest {
	    storage_id?: string;
	    large_unknown_minimum?: number;
	
	    static createFrom(source: any = {}) {
	        return new DiagnoseFormatsRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.storage_id = source["storage_id"];
	        this.large_unknown_minimum = source["large_unknown_minimum"];
	    }
	}

	export class DiagnoseGovernanceRequest {
	    storage_id?: string;
	    large_media_minimum?: number;
	
	    static createFrom(source: any = {}) {
	        return new DiagnoseGovernanceRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.storage_id = source["storage_id"];
	        this.large_media_minimum = source["large_media_minimum"];
	    }
	}

	export class DiagnoseMergesRequest {
	    storage_id?: string;
	
	    static createFrom(source: any = {}) {
	        return new DiagnoseMergesRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.storage_id = source["storage_id"];
	    }
	}

	// Format diagnostic report types (mirror formatdiag package)
	export class FormatDiagSummary {
	    files: number;
	    format_rows: number;
	    missing_format_rows: number;
	    large_unknown: number;
	    extension_mismatches: number;
	    formats_with_metadata_gap: number;
	
	    static createFrom(source: any = {}) {
	        return new FormatDiagSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.files = source["files"];
	        this.format_rows = source["format_rows"];
	        this.missing_format_rows = source["missing_format_rows"];
	        this.large_unknown = source["large_unknown"];
	        this.extension_mismatches = source["extension_mismatches"];
	        this.formats_with_metadata_gap = source["formats_with_metadata_gap"];
	    }
	}

	export class FormatDiagReport {
	    generated_at: string;
	    large_unknown_minimum: number;
	    summary: FormatDiagSummary;
	    large_unknown: any[];
	    extension_mismatches: any[];
	    metadata_gaps: any[];
	    safety_notes: string[];
	
	    static createFrom(source: any = {}) {
	        return new FormatDiagReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.generated_at = source["generated_at"];
	        this.large_unknown_minimum = source["large_unknown_minimum"];
	        this.summary = this.convertValues(source["summary"], FormatDiagSummary);
	        this.large_unknown = source["large_unknown"] || [];
	        this.extension_mismatches = source["extension_mismatches"] || [];
	        this.metadata_gaps = source["metadata_gaps"] || [];
	        this.safety_notes = source["safety_notes"] || [];
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

	// Governance diagnostic report types (mirror governancediag package)
	export class GovernanceDiagSummary {
	    files: number;
	    format_rows: number;
	    missing_format_rows: number;
	    duplicate_groups: number;
	    duplicate_files: number;
	    theoretical_redundant_bytes: number;
	    draft_plans: number;
	    non_draft_plans: number;
	    critical_plans: number;
	    review_actions: number;
	    quarantine_candidate_actions: number;
	    zero_byte_files: number;
	    large_media_files: number;
	    large_media_bytes: number;
	    large_media_with_relations: number;
	    large_media_with_business_anchor: number;
	    large_media_project_work: number;
	    large_media_protected: number;
	    large_media_missing_codec: number;
	    large_media_missing_duration: number;
	    media_relations: number;
	
	    static createFrom(source: any = {}) {
	        return new GovernanceDiagSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.files = source["files"];
	        this.format_rows = source["format_rows"];
	        this.missing_format_rows = source["missing_format_rows"];
	        this.duplicate_groups = source["duplicate_groups"];
	        this.duplicate_files = source["duplicate_files"];
	        this.theoretical_redundant_bytes = source["theoretical_redundant_bytes"];
	        this.draft_plans = source["draft_plans"];
	        this.non_draft_plans = source["non_draft_plans"];
	        this.critical_plans = source["critical_plans"];
	        this.review_actions = source["review_actions"];
	        this.quarantine_candidate_actions = source["quarantine_candidate_actions"];
	        this.zero_byte_files = source["zero_byte_files"];
	        this.large_media_files = source["large_media_files"];
	        this.large_media_bytes = source["large_media_bytes"];
	        this.large_media_with_relations = source["large_media_with_relations"];
	        this.large_media_with_business_anchor = source["large_media_with_business_anchor"];
	        this.large_media_project_work = source["large_media_project_work"];
	        this.large_media_protected = source["large_media_protected"];
	        this.large_media_missing_codec = source["large_media_missing_codec"];
	        this.large_media_missing_duration = source["large_media_missing_duration"];
	        this.media_relations = source["media_relations"];
	    }
	}

	export class GovernanceDiagReport {
	    generated_at: string;
	    large_media_minimum: number;
	    execution_authorized: boolean;
	    summary: GovernanceDiagSummary;
	    duplicate_reviews: any[];
	    zero_byte_reviews: any[];
	    media_aggregates: any[];
	    large_media_reviews: any[];
	    media_relations: any[];
	    safety_notes: string[];
	
	    static createFrom(source: any = {}) {
	        return new GovernanceDiagReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.generated_at = source["generated_at"];
	        this.large_media_minimum = source["large_media_minimum"];
	        this.execution_authorized = source["execution_authorized"];
	        this.summary = this.convertValues(source["summary"], GovernanceDiagSummary);
	        this.duplicate_reviews = source["duplicate_reviews"] || [];
	        this.zero_byte_reviews = source["zero_byte_reviews"] || [];
	        this.media_aggregates = source["media_aggregates"] || [];
	        this.large_media_reviews = source["large_media_reviews"] || [];
	        this.media_relations = source["media_relations"] || [];
	        this.safety_notes = source["safety_notes"] || [];
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

	// Merge diagnostic report types (mirror merge package)
	export class MergeDiagSummary {
	    files: number;
	    directories: number;
	    sibling_parents: number;
	    sibling_pairs: number;
	    name_similar_pairs: number;
	    positive_overlap_pairs: number;
	    overlap_at_least_0_10: number;
	    overlap_at_least_0_25: number;
	    overlap_at_least_0_50: number;
	    suggestions: number;
	
	    static createFrom(source: any = {}) {
	        return new MergeDiagSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.files = source["files"];
	        this.directories = source["directories"];
	        this.sibling_parents = source["sibling_parents"];
	        this.sibling_pairs = source["sibling_pairs"];
	        this.name_similar_pairs = source["name_similar_pairs"];
	        this.positive_overlap_pairs = source["positive_overlap_pairs"];
	        this.overlap_at_least_0_10 = source["overlap_at_least_0_10"];
	        this.overlap_at_least_0_25 = source["overlap_at_least_0_25"];
	        this.overlap_at_least_0_50 = source["overlap_at_least_0_50"];
	        this.suggestions = source["suggestions"];
	    }
	}

	export class MergeDiagReport {
	    generated_at: string;
	    execution_authorized: boolean;
	    suggestion_threshold: number;
	    summary: MergeDiagSummary;
	    name_similar_reviews: any[];
	    safety_notes: string[];
	
	    static createFrom(source: any = {}) {
	        return new MergeDiagReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.generated_at = source["generated_at"];
	        this.execution_authorized = source["execution_authorized"];
	        this.suggestion_threshold = source["suggestion_threshold"];
	        this.summary = this.convertValues(source["summary"], MergeDiagSummary);
	        this.name_similar_reviews = source["name_similar_reviews"] || [];
	        this.safety_notes = source["safety_notes"] || [];
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


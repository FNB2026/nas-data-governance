export namespace domain {
	
	export class ChainNode {
	    path: string;
	    name: string;
	    role: string;
	    authority: number;
	
	    static createFrom(source: any = {}) {
	        return new ChainNode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.role = source["role"];
	        this.authority = source["authority"];
	    }
	}
	export class DirectoryContext {
	    role: string;
	    authority_level: number;
	    privacy_level: string;
	    protected: boolean;
	    matched_terms?: string[];
	    parent_chain?: ChainNode[];
	    branch_point?: string;
	    business_anchor?: string;
	
	    static createFrom(source: any = {}) {
	        return new DirectoryContext(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.authority_level = source["authority_level"];
	        this.privacy_level = source["privacy_level"];
	        this.protected = source["protected"];
	        this.matched_terms = source["matched_terms"];
	        this.parent_chain = this.convertValues(source["parent_chain"], ChainNode);
	        this.branch_point = source["branch_point"];
	        this.business_anchor = source["business_anchor"];
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
	export class FormatInfo {
	    format?: string;
	    category?: string;
	    mime?: string;
	    width?: number;
	    height?: number;
	    duration?: number;
	    pages?: number;
	    codec?: string;
	    archive_entry_count?: number;
	    role?: string;
	    protected?: boolean;
	    regenerable?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FormatInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.format = source["format"];
	        this.category = source["category"];
	        this.mime = source["mime"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.duration = source["duration"];
	        this.pages = source["pages"];
	        this.codec = source["codec"];
	        this.archive_entry_count = source["archive_entry_count"];
	        this.role = source["role"];
	        this.protected = source["protected"];
	        this.regenerable = source["regenerable"];
	    }
	}
	export class PhysicalIdentity {
	    device: number;
	    inode: number;
	    link_count?: number;
	    reliable: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PhysicalIdentity(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.device = source["device"];
	        this.inode = source["inode"];
	        this.link_count = source["link_count"];
	        this.reliable = source["reliable"];
	    }
	}
	export class FileInstance {
	    storage_id: string;
	    path: string;
	    name: string;
	    size: number;
	    mode: number;
	    // Go type: time
	    modified_at: any;
	    device: number;
	    inode: number;
	    is_symlink: boolean;
	    quick_hash?: string;
	    content_sha256?: string;
	    // Go type: time
	    discovered_at: any;
	    physical?: PhysicalIdentity;
	    format?: FormatInfo;
	
	    static createFrom(source: any = {}) {
	        return new FileInstance(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.storage_id = source["storage_id"];
	        this.path = source["path"];
	        this.name = source["name"];
	        this.size = source["size"];
	        this.mode = source["mode"];
	        this.modified_at = this.convertValues(source["modified_at"], null);
	        this.device = source["device"];
	        this.inode = source["inode"];
	        this.is_symlink = source["is_symlink"];
	        this.quick_hash = source["quick_hash"];
	        this.content_sha256 = source["content_sha256"];
	        this.discovered_at = this.convertValues(source["discovered_at"], null);
	        this.physical = this.convertValues(source["physical"], PhysicalIdentity);
	        this.format = this.convertValues(source["format"], FormatInfo);
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
	export class FileRelation {
	    type: string;
	    a: string;
	    b: string;
	    score?: number;
	    evidence?: string[];
	
	    static createFrom(source: any = {}) {
	        return new FileRelation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.a = source["a"];
	        this.b = source["b"];
	        this.score = source["score"];
	        this.evidence = source["evidence"];
	    }
	}
	
	export class PlannedAction {
	    path: string;
	    action: string;
	    reason: string;
	    target_path?: string;
	    context: DirectoryContext;
	    file?: FileInstance;
	
	    static createFrom(source: any = {}) {
	        return new PlannedAction(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.action = source["action"];
	        this.reason = source["reason"];
	        this.target_path = source["target_path"];
	        this.context = this.convertValues(source["context"], DirectoryContext);
	        this.file = this.convertValues(source["file"], FileInstance);
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
	export class RetentionScore {
	    total: number;
	    authority: number;
	    stability: number;
	    path_depth: number;
	    role_bonus: number;
	    reasons: string[];
	
	    static createFrom(source: any = {}) {
	        return new RetentionScore(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total = source["total"];
	        this.authority = source["authority"];
	        this.stability = source["stability"];
	        this.path_depth = source["path_depth"];
	        this.role_bonus = source["role_bonus"];
	        this.reasons = source["reasons"];
	    }
	}
	export class OperationPlan {
	    id: string;
	    group_id?: string;
	    task_id?: string;
	    state: string;
	    content_sha256: string;
	    size: number;
	    risk: string;
	    retain_path?: string;
	    retain_score?: RetentionScore;
	    actions: PlannedAction[];
	    evidence: string[];
	
	    static createFrom(source: any = {}) {
	        return new OperationPlan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.group_id = source["group_id"];
	        this.task_id = source["task_id"];
	        this.state = source["state"];
	        this.content_sha256 = source["content_sha256"];
	        this.size = source["size"];
	        this.risk = source["risk"];
	        this.retain_path = source["retain_path"];
	        this.retain_score = this.convertValues(source["retain_score"], RetentionScore);
	        this.actions = this.convertValues(source["actions"], PlannedAction);
	        this.evidence = source["evidence"];
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

export namespace formatdiag {
	
	export class ExtensionMismatch {
	    storage_id: string;
	    path: string;
	    size: number;
	    extension: string;
	    expected: string[];
	    detected: string;
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new ExtensionMismatch(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.storage_id = source["storage_id"];
	        this.path = source["path"];
	        this.size = source["size"];
	        this.extension = source["extension"];
	        this.expected = source["expected"];
	        this.detected = source["detected"];
	        this.reason = source["reason"];
	    }
	}
	export class MetadataGap {
	    format: string;
	    category: string;
	    missing_duration: number;
	    missing_dimensions: number;
	    bytes: number;
	
	    static createFrom(source: any = {}) {
	        return new MetadataGap(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.format = source["format"];
	        this.category = source["category"];
	        this.missing_duration = source["missing_duration"];
	        this.missing_dimensions = source["missing_dimensions"];
	        this.bytes = source["bytes"];
	    }
	}
	export class ReviewItem {
	    storage_id: string;
	    path: string;
	    size: number;
	    // Go type: time
	    modified_at: any;
	    quick_hash?: string;
	    content_sha256?: string;
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new ReviewItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.storage_id = source["storage_id"];
	        this.path = source["path"];
	        this.size = source["size"];
	        this.modified_at = this.convertValues(source["modified_at"], null);
	        this.quick_hash = source["quick_hash"];
	        this.content_sha256 = source["content_sha256"];
	        this.reason = source["reason"];
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
	export class Summary {
	    files: number;
	    format_rows: number;
	    missing_format_rows: number;
	    large_unknown: number;
	    extension_mismatches: number;
	    formats_with_metadata_gap: number;
	
	    static createFrom(source: any = {}) {
	        return new Summary(source);
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
	export class Report {
	    // Go type: time
	    generated_at: any;
	    large_unknown_minimum: number;
	    summary: Summary;
	    large_unknown: ReviewItem[];
	    extension_mismatches: ExtensionMismatch[];
	    metadata_gaps: MetadataGap[];
	    safety_notes: string[];
	
	    static createFrom(source: any = {}) {
	        return new Report(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.generated_at = this.convertValues(source["generated_at"], null);
	        this.large_unknown_minimum = source["large_unknown_minimum"];
	        this.summary = this.convertValues(source["summary"], Summary);
	        this.large_unknown = this.convertValues(source["large_unknown"], ReviewItem);
	        this.extension_mismatches = this.convertValues(source["extension_mismatches"], ExtensionMismatch);
	        this.metadata_gaps = this.convertValues(source["metadata_gaps"], MetadataGap);
	        this.safety_notes = source["safety_notes"];
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

export namespace governancediag {
	
	export class DuplicateReview {
	    sha256: string;
	    size: number;
	    copies: number;
	    redundant_bytes: number;
	    draft_plan: domain.OperationPlan;
	
	    static createFrom(source: any = {}) {
	        return new DuplicateReview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sha256 = source["sha256"];
	        this.size = source["size"];
	        this.copies = source["copies"];
	        this.redundant_bytes = source["redundant_bytes"];
	        this.draft_plan = this.convertValues(source["draft_plan"], domain.OperationPlan);
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
	export class LargeMediaReview {
	    storage_id: string;
	    path: string;
	    size: number;
	    format: domain.FormatInfo;
	    context: domain.DirectoryContext;
	    relation_count: number;
	    recommendation: string;
	    evidence: string[];
	
	    static createFrom(source: any = {}) {
	        return new LargeMediaReview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.storage_id = source["storage_id"];
	        this.path = source["path"];
	        this.size = source["size"];
	        this.format = this.convertValues(source["format"], domain.FormatInfo);
	        this.context = this.convertValues(source["context"], domain.DirectoryContext);
	        this.relation_count = source["relation_count"];
	        this.recommendation = source["recommendation"];
	        this.evidence = source["evidence"];
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
	export class MediaAggregate {
	    format: string;
	    codec: string;
	    files: number;
	    bytes: number;
	    large_files: number;
	    missing_duration: number;
	    missing_dimensions: number;
	
	    static createFrom(source: any = {}) {
	        return new MediaAggregate(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.format = source["format"];
	        this.codec = source["codec"];
	        this.files = source["files"];
	        this.bytes = source["bytes"];
	        this.large_files = source["large_files"];
	        this.missing_duration = source["missing_duration"];
	        this.missing_dimensions = source["missing_dimensions"];
	    }
	}
	export class ZeroByteReview {
	    storage_id: string;
	    path: string;
	    classification: string;
	    context: domain.DirectoryContext;
	    evidence: string[];
	    recommendation: string;
	
	    static createFrom(source: any = {}) {
	        return new ZeroByteReview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.storage_id = source["storage_id"];
	        this.path = source["path"];
	        this.classification = source["classification"];
	        this.context = this.convertValues(source["context"], domain.DirectoryContext);
	        this.evidence = source["evidence"];
	        this.recommendation = source["recommendation"];
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
	export class Summary {
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
	        return new Summary(source);
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
	export class Report {
	    // Go type: time
	    generated_at: any;
	    large_media_minimum: number;
	    execution_authorized: boolean;
	    summary: Summary;
	    duplicate_reviews: DuplicateReview[];
	    zero_byte_reviews: ZeroByteReview[];
	    media_aggregates: MediaAggregate[];
	    large_media_reviews: LargeMediaReview[];
	    media_relations: domain.FileRelation[];
	    safety_notes: string[];
	
	    static createFrom(source: any = {}) {
	        return new Report(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.generated_at = this.convertValues(source["generated_at"], null);
	        this.large_media_minimum = source["large_media_minimum"];
	        this.execution_authorized = source["execution_authorized"];
	        this.summary = this.convertValues(source["summary"], Summary);
	        this.duplicate_reviews = this.convertValues(source["duplicate_reviews"], DuplicateReview);
	        this.zero_byte_reviews = this.convertValues(source["zero_byte_reviews"], ZeroByteReview);
	        this.media_aggregates = this.convertValues(source["media_aggregates"], MediaAggregate);
	        this.large_media_reviews = this.convertValues(source["large_media_reviews"], LargeMediaReview);
	        this.media_relations = this.convertValues(source["media_relations"], domain.FileRelation);
	        this.safety_notes = source["safety_notes"];
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

export namespace merge {
	
	export class PairReview {
	    directory_a: string;
	    directory_b: string;
	    filename_jaccard: number;
	    gate: string;
	    evidence: string[];
	
	    static createFrom(source: any = {}) {
	        return new PairReview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.directory_a = source["directory_a"];
	        this.directory_b = source["directory_b"];
	        this.filename_jaccard = source["filename_jaccard"];
	        this.gate = source["gate"];
	        this.evidence = source["evidence"];
	    }
	}
	export class DiagnosticSummary {
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
	        return new DiagnosticSummary(source);
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
	export class DiagnosticReport {
	    // Go type: time
	    generated_at: any;
	    execution_authorized: boolean;
	    suggestion_threshold: number;
	    summary: DiagnosticSummary;
	    name_similar_reviews: PairReview[];
	    safety_notes: string[];
	
	    static createFrom(source: any = {}) {
	        return new DiagnosticReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.generated_at = this.convertValues(source["generated_at"], null);
	        this.execution_authorized = source["execution_authorized"];
	        this.suggestion_threshold = source["suggestion_threshold"];
	        this.summary = this.convertValues(source["summary"], DiagnosticSummary);
	        this.name_similar_reviews = this.convertValues(source["name_similar_reviews"], PairReview);
	        this.safety_notes = source["safety_notes"];
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

export namespace wails {
	
	export class AppCapabilitiesDTO {
	    project_open: boolean;
	    project_mode: string;
	    can_scan: boolean;
	    can_view_results: boolean;
	    can_edit_reviews: boolean;
	    can_approve_plans: boolean;
	    can_execute_quarantine: boolean;
	    can_execute_purge: boolean;
	    recovery_lock_active: boolean;
	    disabled_reasons: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new AppCapabilitiesDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.project_open = source["project_open"];
	        this.project_mode = source["project_mode"];
	        this.can_scan = source["can_scan"];
	        this.can_view_results = source["can_view_results"];
	        this.can_edit_reviews = source["can_edit_reviews"];
	        this.can_approve_plans = source["can_approve_plans"];
	        this.can_execute_quarantine = source["can_execute_quarantine"];
	        this.can_execute_purge = source["can_execute_purge"];
	        this.recovery_lock_active = source["recovery_lock_active"];
	        this.disabled_reasons = source["disabled_reasons"];
	    }
	}
	export class ApprovePlansRequest {
	    plan_ids: string[];
	
	    static createFrom(source: any = {}) {
	        return new ApprovePlansRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.plan_ids = source["plan_ids"];
	    }
	}
	export class PlanActionDTO {
	    path: string;
	    action: string;
	    reason: string;
	    target_path?: string;
	    context_role?: string;
	
	    static createFrom(source: any = {}) {
	        return new PlanActionDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.action = source["action"];
	        this.reason = source["reason"];
	        this.target_path = source["target_path"];
	        this.context_role = source["context_role"];
	    }
	}
	export class PlanDTO {
	    id: string;
	    group_id: string;
	    task_id?: string;
	    state: string;
	    content_sha256: string;
	    size: number;
	    risk: string;
	    retain_path?: string;
	    actions: PlanActionDTO[];
	    evidence: string[];
	
	    static createFrom(source: any = {}) {
	        return new PlanDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.group_id = source["group_id"];
	        this.task_id = source["task_id"];
	        this.state = source["state"];
	        this.content_sha256 = source["content_sha256"];
	        this.size = source["size"];
	        this.risk = source["risk"];
	        this.retain_path = source["retain_path"];
	        this.actions = this.convertValues(source["actions"], PlanActionDTO);
	        this.evidence = source["evidence"];
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
	export class ApprovePlansResponse {
	    approved: PlanDTO[];
	
	    static createFrom(source: any = {}) {
	        return new ApprovePlansResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.approved = this.convertValues(source["approved"], PlanDTO);
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
	export class CreateProjectInput {
	    name: string;
	    source_path: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateProjectInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.source_path = source["source_path"];
	    }
	}
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
	export class ExecutionStepDTO {
	    name: string;
	    status: string;
	    detail?: Record<string, any>;
	
	    static createFrom(source: any = {}) {
	        return new ExecutionStepDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.status = source["status"];
	        this.detail = source["detail"];
	    }
	}
	export class ExecutePlanResultDTO {
	    plan_id: string;
	    final_state: string;
	    steps: ExecutionStepDTO[];
	    error_type?: string;
	
	    static createFrom(source: any = {}) {
	        return new ExecutePlanResultDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.plan_id = source["plan_id"];
	        this.final_state = source["final_state"];
	        this.steps = this.convertValues(source["steps"], ExecutionStepDTO);
	        this.error_type = source["error_type"];
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
	export class ExecutePlansRequest {
	    plan_ids: string[];
	    quarantine_root: string;
	    source_roots: string[];
	    dry_run: boolean;
	    retention_hours: number;
	
	    static createFrom(source: any = {}) {
	        return new ExecutePlansRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.plan_ids = source["plan_ids"];
	        this.quarantine_root = source["quarantine_root"];
	        this.source_roots = source["source_roots"];
	        this.dry_run = source["dry_run"];
	        this.retention_hours = source["retention_hours"];
	    }
	}
	export class ExecutePlansResponse {
	    results: ExecutePlanResultDTO[];
	    executed: number;
	    skipped: number;
	    failed: number;
	
	    static createFrom(source: any = {}) {
	        return new ExecutePlansResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.results = this.convertValues(source["results"], ExecutePlanResultDTO);
	        this.executed = source["executed"];
	        this.skipped = source["skipped"];
	        this.failed = source["failed"];
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
	export class ExecutePurgeRequest {
	    plan_id: string;
	    digest: string;
	    quarantine_root: string;
	    dry_run: boolean;
	    confirmation?: string;
	
	    static createFrom(source: any = {}) {
	        return new ExecutePurgeRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.plan_id = source["plan_id"];
	        this.digest = source["digest"];
	        this.quarantine_root = source["quarantine_root"];
	        this.dry_run = source["dry_run"];
	        this.confirmation = source["confirmation"];
	    }
	}
	export class ExecutePurgeResponse {
	    plan_id: string;
	    final_state: string;
	    status: string;
	    error_type?: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ExecutePurgeResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.plan_id = source["plan_id"];
	        this.final_state = source["final_state"];
	        this.status = source["status"];
	        this.error_type = source["error_type"];
	        this.error = source["error"];
	    }
	}
	export class ExecuteRestoreRequest {
	    plan_id: string;
	    digest: string;
	    quarantine_root: string;
	    source_roots: string[];
	    dry_run: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ExecuteRestoreRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.plan_id = source["plan_id"];
	        this.digest = source["digest"];
	        this.quarantine_root = source["quarantine_root"];
	        this.source_roots = source["source_roots"];
	        this.dry_run = source["dry_run"];
	    }
	}
	export class ExecuteRestoreResponse {
	    plan_id: string;
	    final_state: string;
	    status: string;
	    error_type?: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ExecuteRestoreResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.plan_id = source["plan_id"];
	        this.final_state = source["final_state"];
	        this.status = source["status"];
	        this.error_type = source["error_type"];
	        this.error = source["error"];
	    }
	}
	
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
	export class GroupDecisionDTO {
	    id: string;
	    group_id: string;
	    decision_type: string;
	    retained_file_id?: number;
	    reason?: string;
	    rule_id?: string;
	    created_at: string;
	    updated_at: string;
	
	    static createFrom(source: any = {}) {
	        return new GroupDecisionDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.group_id = source["group_id"];
	        this.decision_type = source["decision_type"];
	        this.retained_file_id = source["retained_file_id"];
	        this.reason = source["reason"];
	        this.rule_id = source["rule_id"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
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
	export class JournalEntryDTO {
	    plan_id: string;
	    task_id: string;
	    action_index: number;
	    action_type: string;
	    source_path: string;
	    target_path?: string;
	    content_sha256: string;
	    file_size: number;
	    status: string;
	    rollback_status?: string;
	    started_at?: string;
	    completed_at?: string;
	
	    static createFrom(source: any = {}) {
	        return new JournalEntryDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.plan_id = source["plan_id"];
	        this.task_id = source["task_id"];
	        this.action_index = source["action_index"];
	        this.action_type = source["action_type"];
	        this.source_path = source["source_path"];
	        this.target_path = source["target_path"];
	        this.content_sha256 = source["content_sha256"];
	        this.file_size = source["file_size"];
	        this.status = source["status"];
	        this.rollback_status = source["rollback_status"];
	        this.started_at = source["started_at"];
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
	export class OperationLogDTO {
	    id: number;
	    plan_id: string;
	    event_type: string;
	    detail?: Record<string, any>;
	    created_at: string;
	
	    static createFrom(source: any = {}) {
	        return new OperationLogDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.plan_id = source["plan_id"];
	        this.event_type = source["event_type"];
	        this.detail = source["detail"];
	        this.created_at = source["created_at"];
	    }
	}
	
	
	export class ProjectInfo {
	    project_id: string;
	    name: string;
	    path: string;
	    is_open: boolean;
	    storage_count: number;
	
	    static createFrom(source: any = {}) {
	        return new ProjectInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.project_id = source["project_id"];
	        this.name = source["name"];
	        this.path = source["path"];
	        this.is_open = source["is_open"];
	        this.storage_count = source["storage_count"];
	    }
	}
	export class ReadinessCheckDTO {
	    key: string;
	    label: string;
	    passed: boolean;
	    reason?: string;
	
	    static createFrom(source: any = {}) {
	        return new ReadinessCheckDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.label = source["label"];
	        this.passed = source["passed"];
	        this.reason = source["reason"];
	    }
	}
	export class ProjectReadinessDTO {
	    ready: boolean;
	    checks: ReadinessCheckDTO[];
	    storage_count: number;
	    file_count: number;
	    plan_count: number;
	
	    static createFrom(source: any = {}) {
	        return new ProjectReadinessDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ready = source["ready"];
	        this.checks = this.convertValues(source["checks"], ReadinessCheckDTO);
	        this.storage_count = source["storage_count"];
	        this.file_count = source["file_count"];
	        this.plan_count = source["plan_count"];
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
	export class PurgePlanDTO {
	    id: string;
	    item_id: string;
	    state: string;
	    expected_path: string;
	    expected_sha256: string;
	    expected_size: number;
	    retain_until: string;
	    approval_digest: string;
	    confirmation_text: string;
	    created_at: string;
	    approved_at?: string;
	    dry_run_verified_at?: string;
	    purged_at?: string;
	
	    static createFrom(source: any = {}) {
	        return new PurgePlanDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.item_id = source["item_id"];
	        this.state = source["state"];
	        this.expected_path = source["expected_path"];
	        this.expected_sha256 = source["expected_sha256"];
	        this.expected_size = source["expected_size"];
	        this.retain_until = source["retain_until"];
	        this.approval_digest = source["approval_digest"];
	        this.confirmation_text = source["confirmation_text"];
	        this.created_at = source["created_at"];
	        this.approved_at = source["approved_at"];
	        this.dry_run_verified_at = source["dry_run_verified_at"];
	        this.purged_at = source["purged_at"];
	    }
	}
	export class PurgeRecoveryResultDTO {
	    plan_id?: string;
	    final_state?: string;
	    status: string;
	    error_type?: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new PurgeRecoveryResultDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.plan_id = source["plan_id"];
	        this.final_state = source["final_state"];
	        this.status = source["status"];
	        this.error_type = source["error_type"];
	        this.error = source["error"];
	    }
	}
	export class QuarantineItemDTO {
	    id: string;
	    plan_id: string;
	    action_index: number;
	    source_path: string;
	    quarantine_path: string;
	    content_sha256: string;
	    file_size: number;
	    quarantined_at: string;
	    retain_until: string;
	    status: string;
	    hold_reason?: string;
	    restored_at?: string;
	    purged_at?: string;
	
	    static createFrom(source: any = {}) {
	        return new QuarantineItemDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.plan_id = source["plan_id"];
	        this.action_index = source["action_index"];
	        this.source_path = source["source_path"];
	        this.quarantine_path = source["quarantine_path"];
	        this.content_sha256 = source["content_sha256"];
	        this.file_size = source["file_size"];
	        this.quarantined_at = source["quarantined_at"];
	        this.retain_until = source["retain_until"];
	        this.status = source["status"];
	        this.hold_reason = source["hold_reason"];
	        this.restored_at = source["restored_at"];
	        this.purged_at = source["purged_at"];
	    }
	}
	
	export class RecentProjectEntry {
	    name: string;
	    path: string;
	    // Go type: time
	    opened_at: any;
	
	    static createFrom(source: any = {}) {
	        return new RecentProjectEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.opened_at = this.convertValues(source["opened_at"], null);
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
	export class RecoverRestoresRequest {
	    quarantine_root: string;
	    source_roots: string[];
	
	    static createFrom(source: any = {}) {
	        return new RecoverRestoresRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.quarantine_root = source["quarantine_root"];
	        this.source_roots = source["source_roots"];
	    }
	}
	export class RecoveryResultDTO {
	    plan_id: string;
	    action: string;
	    rolled_back: number;
	    errors?: string[];
	
	    static createFrom(source: any = {}) {
	        return new RecoveryResultDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.plan_id = source["plan_id"];
	        this.action = source["action"];
	        this.rolled_back = source["rolled_back"];
	        this.errors = source["errors"];
	    }
	}
	export class RecoveryStatusDTO {
	    lock_active: boolean;
	    executing_count: number;
	    source_executing_count: number;
	    restore_pending_count: number;
	    purge_recoverable_count: number;
	
	    static createFrom(source: any = {}) {
	        return new RecoveryStatusDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.lock_active = source["lock_active"];
	        this.executing_count = source["executing_count"];
	        this.source_executing_count = source["source_executing_count"];
	        this.restore_pending_count = source["restore_pending_count"];
	        this.purge_recoverable_count = source["purge_recoverable_count"];
	    }
	}
	export class RestorePlanDTO {
	    id: string;
	    item_id: string;
	    state: string;
	    quarantine_path: string;
	    restore_path: string;
	    expected_sha256: string;
	    expected_size: number;
	    approval_digest: string;
	    created_at: string;
	    approved_at?: string;
	    restored_at?: string;
	
	    static createFrom(source: any = {}) {
	        return new RestorePlanDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.item_id = source["item_id"];
	        this.state = source["state"];
	        this.quarantine_path = source["quarantine_path"];
	        this.restore_path = source["restore_path"];
	        this.expected_sha256 = source["expected_sha256"];
	        this.expected_size = source["expected_size"];
	        this.approval_digest = source["approval_digest"];
	        this.created_at = source["created_at"];
	        this.approved_at = source["approved_at"];
	        this.restored_at = source["restored_at"];
	    }
	}
	export class RestoreRecoveryResultDTO {
	    plan_id?: string;
	    final_state?: string;
	    status: string;
	    error_type?: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new RestoreRecoveryResultDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.plan_id = source["plan_id"];
	        this.final_state = source["final_state"];
	        this.status = source["status"];
	        this.error_type = source["error_type"];
	        this.error = source["error"];
	    }
	}
	export class SaveDecisionRequest {
	    group_id: string;
	    decision_type: string;
	    reason?: string;
	
	    static createFrom(source: any = {}) {
	        return new SaveDecisionRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.group_id = source["group_id"];
	        this.decision_type = source["decision_type"];
	        this.reason = source["reason"];
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


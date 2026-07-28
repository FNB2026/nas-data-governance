export namespace wails {

	export class FileItem {
	    storage_id: string;
	    path: string;
	    name: string;
	    size: number;
	    modified_at: string;
	    is_symlink: boolean;
	    quick_hash: string;
	    content_sha256: string;
	    physical_device: number;
	    physical_inode: number;
	    physical_link_count: number;
	    physical_reliable: boolean;
	    format_kind: string;
	    format_mime: string;

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
	    decision_type: string;
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
	        this.files = source["files"]?.map((item: any) => FileItem.createFrom(item));
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
	    decision_type: string;

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
	export class ListGroupsRequest {
	    storage_id: string;
	    page_size: number;
	    cursor: string;
	    min_reclaimable_bytes: number;

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
	    next_cursor: string;
	    total_count: number;

	    static createFrom(source: any = {}) {
	        return new ListGroupsResponse(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.groups = source["groups"]?.map((item: any) => GroupSummary.createFrom(item));
	        this.next_cursor = source["next_cursor"];
	        this.total_count = source["total_count"];
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

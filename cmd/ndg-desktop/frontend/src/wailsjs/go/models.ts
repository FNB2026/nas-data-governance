export namespace wails {
	
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


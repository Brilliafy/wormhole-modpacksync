export namespace models {
	
	export class FileTreeNode {
	    id: string;
	    name: string;
	    relPath: string;
	    isDir: boolean;
	    size: number;
	    checked: boolean;
	    children?: FileTreeNode[];
	
	    static createFrom(source: any = {}) {
	        return new FileTreeNode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.relPath = source["relPath"];
	        this.isDir = source["isDir"];
	        this.size = source["size"];
	        this.checked = source["checked"];
	        this.children = this.convertValues(source["children"], FileTreeNode);
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
	export class NetworkInfo {
	    localIPs: string[];
	    tailscaleIP: string;
	    defaultPort: number;
	
	    static createFrom(source: any = {}) {
	        return new NetworkInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.localIPs = source["localIPs"];
	        this.tailscaleIP = source["tailscaleIP"];
	        this.defaultPort = source["defaultPort"];
	    }
	}
	export class SenderInitResult {
	    code: string;
	    directAddresses: string[];
	    tailscaleIP: string;
	    selectedFiles: number;
	    totalBytes: number;
	
	    static createFrom(source: any = {}) {
	        return new SenderInitResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.code = source["code"];
	        this.directAddresses = source["directAddresses"];
	        this.tailscaleIP = source["tailscaleIP"];
	        this.selectedFiles = source["selectedFiles"];
	        this.totalBytes = source["totalBytes"];
	    }
	}
	export class SyncStats {
	    status: string;
	    role: string;
	    mode: string;
	    totalFiles: number;
	    completedFiles: number;
	    totalBytes: number;
	    transferredBytes: number;
	    speedBytesPerSec: number;
	    speedFormatted: string;
	    percent: number;
	    currentFile: string;
	    message: string;
	    errorMessage: string;
	    code: string;
	    peerAddress: string;
	
	    static createFrom(source: any = {}) {
	        return new SyncStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.role = source["role"];
	        this.mode = source["mode"];
	        this.totalFiles = source["totalFiles"];
	        this.completedFiles = source["completedFiles"];
	        this.totalBytes = source["totalBytes"];
	        this.transferredBytes = source["transferredBytes"];
	        this.speedBytesPerSec = source["speedBytesPerSec"];
	        this.speedFormatted = source["speedFormatted"];
	        this.percent = source["percent"];
	        this.currentFile = source["currentFile"];
	        this.message = source["message"];
	        this.errorMessage = source["errorMessage"];
	        this.code = source["code"];
	        this.peerAddress = source["peerAddress"];
	    }
	}

}


export namespace config {
	
	export class Config {
	    server_url: string;
	    token: string;
	    machine_id: string;
	    machine_name: string;
	    sync_interval: number;
	    path_mappings: {[key: string]: string};
	    auto_start: boolean;
	    paused: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.server_url = source["server_url"];
	        this.token = source["token"];
	        this.machine_id = source["machine_id"];
	        this.machine_name = source["machine_name"];
	        this.sync_interval = source["sync_interval"];
	        this.path_mappings = source["path_mappings"];
	        this.auto_start = source["auto_start"];
	        this.paused = source["paused"];
	    }
	}

}


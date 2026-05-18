export namespace db {
	
	export class AssetPageResult {
	    items: model.Asset[];
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new AssetPageResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], model.Asset);
	        this.total = source["total"];
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
	
	export class Asset {
	    id: number;
	    project_id: number;
	    type: string;
	    host: string;
	    port: string;
	    sources: string[];
	    tags: string[];
	    status: string;
	    status_code?: number;
	    title: string;
	    server: string;
	    tech: string;
	    probed_at: string;
	    created_at: string;
	
	    static createFrom(source: any = {}) {
	        return new Asset(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.project_id = source["project_id"];
	        this.type = source["type"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.sources = source["sources"];
	        this.tags = source["tags"];
	        this.status = source["status"];
	        this.status_code = source["status_code"];
	        this.title = source["title"];
	        this.server = source["server"];
	        this.tech = source["tech"];
	        this.probed_at = source["probed_at"];
	        this.created_at = source["created_at"];
	    }
	}
	export class ImportResult {
	    new_ip: number;
	    new_domain: number;
	    skipped: number;
	
	    static createFrom(source: any = {}) {
	        return new ImportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.new_ip = source["new_ip"];
	        this.new_domain = source["new_domain"];
	        this.skipped = source["skipped"];
	    }
	}
	export class Project {
	    id: number;
	    name: string;
	    created_at: string;
	    asset_count: number;
	    ip_count: number;
	    domain_count: number;
	    alive_count: number;
	
	    static createFrom(source: any = {}) {
	        return new Project(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.created_at = source["created_at"];
	        this.asset_count = source["asset_count"];
	        this.ip_count = source["ip_count"];
	        this.domain_count = source["domain_count"];
	        this.alive_count = source["alive_count"];
	    }
	}

}

export namespace scanner {
	
	export class DnsConfig {
	    concurrency: number;
	    timeout: number;
	    dns_server: string;
	
	    static createFrom(source: any = {}) {
	        return new DnsConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.concurrency = source["concurrency"];
	        this.timeout = source["timeout"];
	        this.dns_server = source["dns_server"];
	    }
	}
	export class HttpxConfig {
	    httpx_path: string;
	    threads: number;
	    timeout: number;
	    retries: number;
	    rate_limit: number;
	    probe_title: boolean;
	    probe_tech: boolean;
	    probe_server: boolean;
	    probe_content_length: boolean;
	    probe_ip: boolean;
	    probe_cdn: boolean;
	    follow_redirects: boolean;
	    match_codes: string;
	    filter_codes: string;
	    only_unprobed: boolean;
	    only_ip: boolean;
	    only_domain: boolean;
	    skip_dns_failed: boolean;
	
	    static createFrom(source: any = {}) {
	        return new HttpxConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.httpx_path = source["httpx_path"];
	        this.threads = source["threads"];
	        this.timeout = source["timeout"];
	        this.retries = source["retries"];
	        this.rate_limit = source["rate_limit"];
	        this.probe_title = source["probe_title"];
	        this.probe_tech = source["probe_tech"];
	        this.probe_server = source["probe_server"];
	        this.probe_content_length = source["probe_content_length"];
	        this.probe_ip = source["probe_ip"];
	        this.probe_cdn = source["probe_cdn"];
	        this.follow_redirects = source["follow_redirects"];
	        this.match_codes = source["match_codes"];
	        this.filter_codes = source["filter_codes"];
	        this.only_unprobed = source["only_unprobed"];
	        this.only_ip = source["only_ip"];
	        this.only_domain = source["only_domain"];
	        this.skip_dns_failed = source["skip_dns_failed"];
	    }
	}
	export class NaabuConfig {
	    naabu_path: string;
	    ports: string;
	    rate: number;
	    concurrency: number;
	    timeout: number;
	    retries: number;
	    scan_type: string;
	    exclude_cdn: boolean;
	    verify: boolean;
	    only_ip: boolean;
	    only_domain: boolean;
	    only_alive: boolean;
	    skip_dns_failed: boolean;
	    use_ip_ports: boolean;
	
	    static createFrom(source: any = {}) {
	        return new NaabuConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.naabu_path = source["naabu_path"];
	        this.ports = source["ports"];
	        this.rate = source["rate"];
	        this.concurrency = source["concurrency"];
	        this.timeout = source["timeout"];
	        this.retries = source["retries"];
	        this.scan_type = source["scan_type"];
	        this.exclude_cdn = source["exclude_cdn"];
	        this.verify = source["verify"];
	        this.only_ip = source["only_ip"];
	        this.only_domain = source["only_domain"];
	        this.only_alive = source["only_alive"];
	        this.skip_dns_failed = source["skip_dns_failed"];
	        this.use_ip_ports = source["use_ip_ports"];
	    }
	}
	export class RustscanConfig {
	    rustscan_path: string;
	    ports: string;
	    ulimit: number;
	    batch_size: number;
	    timeout: number;
	    tries: number;
	    no_cdn: boolean;
	    only_ip: boolean;
	    only_domain: boolean;
	    only_alive: boolean;
	    no_banner: boolean;
	    skip_dns_failed: boolean;
	    use_ip_ports: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RustscanConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rustscan_path = source["rustscan_path"];
	        this.ports = source["ports"];
	        this.ulimit = source["ulimit"];
	        this.batch_size = source["batch_size"];
	        this.timeout = source["timeout"];
	        this.tries = source["tries"];
	        this.no_cdn = source["no_cdn"];
	        this.only_ip = source["only_ip"];
	        this.only_domain = source["only_domain"];
	        this.only_alive = source["only_alive"];
	        this.no_banner = source["no_banner"];
	        this.skip_dns_failed = source["skip_dns_failed"];
	        this.use_ip_ports = source["use_ip_ports"];
	    }
	}
	export class SubdomainConfig {
	    tool: string;
	    tool_path: string;
	    python_path: string;
	    domains: string[];
	    threads: number;
	    timeout: number;
	    all: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SubdomainConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tool = source["tool"];
	        this.tool_path = source["tool_path"];
	        this.python_path = source["python_path"];
	        this.domains = source["domains"];
	        this.threads = source["threads"];
	        this.timeout = source["timeout"];
	        this.all = source["all"];
	    }
	}

}


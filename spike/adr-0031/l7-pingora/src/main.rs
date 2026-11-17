//! L7 for the ADR-0031 spike (branch A1): Pingora proxy with
//! - an in-memory snapshot swapped atomically (ArcSwap), re-read when the file changes (apply class A);
//! - pre-filter (per-zone ACL deny, per-second rate limit) before any WAF work;
//! - WAF callout over gRPC (the Inspection API, simplified), per-zone exclusions, fail mode per zone;
//! - SNI certificate selection from the same snapshot, no reload;
//! - upstream chosen per request from the snapshot.
use std::collections::HashMap;
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::Arc;
use std::time::{Duration, SystemTime};

use arc_swap::ArcSwap;
use async_trait::async_trait;
use bytes::Bytes;
use pingora::listeners::tls::TlsSettings;
use pingora::listeners::TlsAccept;
use pingora::cache::cache_control::CacheControl;
use pingora::cache::filters::resp_cacheable;
use pingora::cache::{CacheKey, CacheMetaDefaults, MemCache, RespCacheable};
use pingora::prelude::*;
use std::sync::LazyLock;
use pingora::tls::pkey::{PKey, Private};
use pingora::tls::ssl::{NameType, SslRef};
use pingora::tls::x509::X509;
use serde::Deserialize;
use tonic::transport::{Channel, Endpoint};

#[global_allocator]
static GLOBAL: tikv_jemallocator::Jemalloc = tikv_jemallocator::Jemalloc;

pub mod pb {
    tonic::include_proto!("inspect.v1");
}
use pb::inspection_client::InspectionClient;

#[derive(Deserialize, Clone, Default)]
struct ZoneCfg {
    host: String,
    id: String,
    origin: String,
    #[serde(default)]
    origins: Vec<String>,
    #[serde(default)]
    acl_deny: Vec<String>,
    #[serde(default)]
    rps: u64,
    #[serde(default)]
    cache: bool,
    #[serde(default)]
    waf: bool,
    #[serde(default)]
    exclude_rule_ids: Vec<u32>,
    #[serde(default = "yes")]
    fail_open: bool,
}
fn yes() -> bool {
    true
}

#[derive(Deserialize, Default)]
struct CertCfg {
    cert: String,
    key: String,
}

#[derive(Deserialize, Default)]
struct SnapshotFile {
    version: u64,
    zones: Vec<ZoneCfg>,
    #[serde(default)]
    certs: HashMap<String, CertCfg>,
}

struct Zone {
    cfg: ZoneCfg,
    origins: Vec<String>,
    healthy: Vec<AtomicBool>,
    pick: AtomicU64,
    window: AtomicU64, // unix second
    hits: AtomicU64,
}

struct Snapshot {
    version: u64,
    zones: HashMap<String, Arc<Zone>>,
    certs: HashMap<String, (X509, PKey<Private>)>,
}

fn load(path: &str) -> Result<Snapshot, String> {
    let raw = std::fs::read_to_string(path).map_err(|e| e.to_string())?;
    let f: SnapshotFile = serde_json::from_str(&raw).map_err(|e| e.to_string())?;
    let mut certs = HashMap::new();
    for (host, c) in f.certs {
        let cert = X509::from_pem(&std::fs::read(&c.cert).map_err(|e| e.to_string())?).map_err(|e| e.to_string())?;
        let key = PKey::private_key_from_pem(&std::fs::read(&c.key).map_err(|e| e.to_string())?).map_err(|e| e.to_string())?;
        certs.insert(host, (cert, key));
    }
    let zones = f
        .zones
        .into_iter()
        .map(|z| {
            let origins = if z.origins.is_empty() { vec![z.origin.clone()] } else { z.origins.clone() };
            let healthy = origins.iter().map(|_| AtomicBool::new(true)).collect();
            (z.host.clone(), Arc::new(Zone { cfg: z, origins, healthy, pick: AtomicU64::new(0), window: AtomicU64::new(0), hits: AtomicU64::new(0) }))
        })
        .collect();
    Ok(Snapshot { version: f.version, zones, certs })
}

struct Shared {
    snap: ArcSwap<Snapshot>,
    waf: Vec<InspectionClient<Channel>>,
    rr: AtomicU64,
    waf_calls: AtomicU64,
    waf_errors: AtomicU64,
}

struct L7(Arc<Shared>);

#[derive(Default)]
struct Ctx {
    zone: Option<Arc<Zone>>,
    body: Vec<u8>,
    has_body: bool,
    tries: u32,
}

/// Short internally generated answer that keeps the client connection alive.
/// `Session::respond_error` closes the downstream connection on purpose (pingora-core's own TODO),
/// which turns every blocked request into a new TCP accept under flood.
async fn deny(session: &mut Session, code: u16) -> Result<()> {
    let mut resp = ResponseHeader::build(code, Some(2))?;
    resp.insert_header("content-length", "0")?;
    session.write_response_header(Box::new(resp), true).await?;
    session.finish_body().await?;
    Ok(())
}

const MAX_BODY: usize = 64 * 1024;
static CACHE: LazyLock<MemCache> = LazyLock::new(MemCache::new);

fn cache_key(req: &RequestHeader) -> CacheKey {
    let host = req.headers.get("host").and_then(|h| h.to_str().ok()).unwrap_or("");
    CacheKey::new(host.to_string(), req.uri.to_string())
}

impl L7 {
    async fn inspect(&self, zone: &Zone, req: &RequestHeader, client_ip: &str, version: u64, body: &[u8]) -> Result<Option<u16>, ()> {
        let sh = &self.0;
        let i = (sh.rr.fetch_add(1, Ordering::Relaxed) as usize) % sh.waf.len();
        let mut cl = sh.waf[i].clone();
        let headers = req
            .headers
            .iter()
            .map(|(k, v)| pb::Header { name: k.as_str().to_string(), value: v.to_str().unwrap_or("").to_string() })
            .collect();
        let msg = pb::InspectRequest {
            zone_id: zone.cfg.id.clone(),
            request_id: String::new(),
            method: req.method.as_str().to_string(),
            uri: req.uri.to_string(),
            http_version: "1.1".into(),
            client_ip: client_ip.to_string(),
            headers,
            body: body.to_vec(),
            policy_version: version,
            deadline_ms: 50,
            exclude_rule_ids: zone.cfg.exclude_rule_ids.clone(),
        };
        let mut r = tonic::Request::new(msg);
        r.set_timeout(Duration::from_millis(50));
        sh.waf_calls.fetch_add(1, Ordering::Relaxed);
        match cl.inspect(r).await {
            Ok(v) => {
                let v = v.into_inner();
                if v.action == pb::verdict::Action::Block as i32 {
                    Ok(Some(if v.status == 0 { 403 } else { v.status as u16 }))
                } else {
                    Ok(None)
                }
            }
            Err(_) => {
                sh.waf_errors.fetch_add(1, Ordering::Relaxed);
                Err(())
            }
        }
    }
}

#[async_trait]
impl ProxyHttp for L7 {
    type CTX = Ctx;
    fn new_ctx(&self) -> Ctx {
        Ctx::default()
    }

    async fn request_filter(&self, session: &mut Session, ctx: &mut Ctx) -> Result<bool> {
        let snap = self.0.snap.load();
        let host = session
            .req_header()
            .headers
            .get("host")
            .and_then(|h| h.to_str().ok())
            .map(|h| h.to_string())
            // HTTP/2 carries the host in :authority, not in a Host header.
            .or_else(|| session.req_header().uri.authority().map(|a| a.to_string()))
            .map(|h| h.split(':').next().unwrap_or(&h).to_string())
            .unwrap_or_default();
        let zone = match snap.zones.get(&host) {
            Some(z) => z.clone(),
            None => {
                deny(session, 404).await?;
                return Ok(true);
            }
        };
        let client_ip = session.client_addr().map(|a| a.to_string()).unwrap_or_default();
        let ip_only = client_ip.rsplit_once(':').map(|(a, _)| a.to_string()).unwrap_or(client_ip.clone());
        // pre-filter: ACL
        if zone.cfg.acl_deny.iter().any(|d| *d == ip_only) {
            deny(session, 403).await?;
            return Ok(true);
        }
        // pre-filter: per-second rate limit (fixed window)
        if zone.cfg.rps > 0 {
            let now = SystemTime::now().duration_since(SystemTime::UNIX_EPOCH).unwrap().as_secs();
            if zone.window.swap(now, Ordering::Relaxed) != now {
                zone.hits.store(0, Ordering::Relaxed);
            }
            if zone.hits.fetch_add(1, Ordering::Relaxed) >= zone.cfg.rps {
                deny(session, 429).await?;
                return Ok(true);
            }
        }
        // Cache purge (class A in ADR-0037): exact URL, per zone.
        if zone.cfg.cache && session.req_header().method.as_str() == "PURGE" {
            let key = cache_key(session.req_header());
            session.cache.enable(&*CACHE, None, None, None, None);
            session.cache.set_cache_key(key);
            let found = session.cache.purge().await.unwrap_or(false);
            session.respond_error(if found { 200 } else { 404 }).await?;
            return Ok(true);
        }
        let has_body = session.req_header().headers.get("content-length").map(|v| v != "0").unwrap_or(false)
            || session.req_header().headers.get("transfer-encoding").is_some();
        let waf = zone.cfg.waf;
        ctx.zone = Some(zone.clone());
        ctx.has_body = has_body;
        // WAF callout for requests without a body happens here; with a body, at end of stream.
        if waf && !has_body {
            let req = session.req_header().clone();
            match self.inspect(&zone, &req, &ip_only, snap.version, &[]).await {
                Ok(Some(st)) => {
                    deny(session, st).await?;
                    return Ok(true);
                }
                Ok(None) => {}
                Err(()) if !zone.cfg.fail_open => {
                    deny(session, 503).await?;
                    return Ok(true);
                }
                Err(()) => {}
            }
        }
        Ok(false)
    }

    fn request_cache_filter(&self, session: &mut Session, ctx: &mut Ctx) -> Result<()> {
        if let Some(z) = ctx.zone.as_ref() {
            if z.cfg.cache && session.req_header().method.as_str() == "GET" {
                session.cache.enable(&*CACHE, None, None, None, None);
            }
        }
        Ok(())
    }

    fn cache_key_callback(&self, session: &Session, _ctx: &mut Ctx) -> Result<CacheKey> {
        Ok(cache_key(session.req_header()))
    }

    fn response_cache_filter(&self, _s: &Session, resp: &ResponseHeader, _ctx: &mut Ctx) -> Result<RespCacheable> {
        static DEFAULTS: CacheMetaDefaults = CacheMetaDefaults::new(|_| Some(Duration::from_secs(60)), 0, 0);
        let cc = CacheControl::from_resp_headers(resp);
        Ok(resp_cacheable(cc.as_ref(), resp.clone(), false, &DEFAULTS))
    }

    async fn request_body_filter(&self, session: &mut Session, body: &mut Option<Bytes>, end: bool, ctx: &mut Ctx) -> Result<()> {
        let zone = match ctx.zone.clone() {
            Some(z) if z.cfg.waf && ctx.has_body => z,
            _ => return Ok(()),
        };
        if let Some(b) = body.as_ref() {
            if ctx.body.len() < MAX_BODY {
                let take = (MAX_BODY - ctx.body.len()).min(b.len());
                ctx.body.extend_from_slice(&b[..take]);
            }
        }
        if end {
            let snap = self.0.snap.load();
            let ip = session.client_addr().map(|a| a.to_string()).unwrap_or_default();
            let ip = ip.rsplit_once(':').map(|(a, _)| a.to_string()).unwrap_or(ip);
            let req = session.req_header().clone();
            match self.inspect(&zone, &req, &ip, snap.version, &ctx.body).await {
                Ok(Some(st)) => return Error::e_explain(HTTPStatus(st), "blocked by waf"),
                Err(()) if !zone.cfg.fail_open => return Error::e_explain(HTTPStatus(503), "waf unavailable"),
                _ => {}
            }
        }
        Ok(())
    }

    async fn upstream_peer(&self, _session: &mut Session, ctx: &mut Ctx) -> Result<Box<HttpPeer>> {
        let z = ctx.zone.as_ref().expect("zone set in request_filter");
        // Round robin over healthy origins; if none is healthy, try them anyway.
        let n = z.origins.len();
        let start = z.pick.fetch_add(1, Ordering::Relaxed) as usize;
        let mut idx = start % n;
        for k in 0..n {
            let c = (start + k) % n;
            if z.healthy[c].load(Ordering::Relaxed) {
                idx = c;
                break;
            }
        }
        ctx.tries += 1;
        Ok(Box::new(HttpPeer::new(z.origins[idx].as_str(), false, String::new())))
    }

    // Passive health + retry: a connect failure marks the origin down and asks the proxy to pick again.
    fn fail_to_connect(&self, _s: &mut Session, peer: &HttpPeer, ctx: &mut Ctx, mut e: Box<Error>) -> Box<Error> {
        if let Some(z) = ctx.zone.as_ref() {
            let addr = peer._address.to_string();
            for (i, o) in z.origins.iter().enumerate() {
                if addr.ends_with(o.as_str()) {
                    z.healthy[i].store(false, Ordering::Relaxed);
                }
            }
        }
        if ctx.tries < 3 {
            e.set_retry(true);
        }
        e
    }
}

struct Certs(Arc<Shared>);

#[async_trait]
impl TlsAccept for Certs {
    async fn certificate_callback(&self, ssl: &mut SslRef) {
        let sni = ssl.servername(NameType::HOST_NAME).unwrap_or("").to_string();
        let snap = self.0.snap.load();
        if let Some((c, k)) = snap.certs.get(&sni) {
            let _ = pingora::tls::ext::ssl_use_certificate(ssl, c);
            let _ = pingora::tls::ext::ssl_use_private_key(ssl, k);
        }
    }
}

fn main() {
    let path = std::env::var("SNAPSHOT").unwrap_or_else(|_| "snapshot.json".into());
    let waf_addr = std::env::var("WAF_ADDR").unwrap_or_else(|_| "http://127.0.0.1:9100".into());
    let snap = load(&path).expect("initial snapshot");
    eprintln!("snapshot v{} loaded: {} zones, {} certs", snap.version, snap.zones.len(), snap.certs.len());

    let rt = tokio::runtime::Builder::new_multi_thread().enable_all().build().unwrap();
    let waf: Vec<_> = (0..4)
        .map(|_| {
            let _g = rt.enter();
            InspectionClient::new(Endpoint::from_shared(waf_addr.clone()).unwrap().connect_lazy())
        })
        .collect();
    let shared = Arc::new(Shared { snap: ArcSwap::from_pointee(snap), waf, rr: AtomicU64::new(0), waf_calls: AtomicU64::new(0), waf_errors: AtomicU64::new(0) });

    // Snapshot watcher: re-read when mtime changes; swap on success, keep the old one on error (NACK).
    {
        let sh = shared.clone();
        let p = path.clone();
        std::thread::spawn(move || {
            let mut last = std::fs::metadata(&p).and_then(|m| m.modified()).ok();
            loop {
                std::thread::sleep(Duration::from_millis(100));
                let m = std::fs::metadata(&p).and_then(|m| m.modified()).ok();
                if m != last {
                    last = m;
                    let t = std::time::Instant::now();
                    match load(&p) {
                        Ok(s) => {
                            let v = s.version;
                            sh.snap.store(Arc::new(s));
                            eprintln!("snapshot v{} applied in {:?}", v, t.elapsed());
                        }
                        Err(e) => eprintln!("snapshot rejected: {e}"),
                    }
                }
            }
        });
    }
    // Active health check: TCP connect to every origin of multi-origin zones once a second.
    {
        let sh = shared.clone();
        std::thread::spawn(move || loop {
            std::thread::sleep(Duration::from_secs(1));
            let snap = sh.snap.load();
            for z in snap.zones.values().filter(|z| z.origins.len() > 1) {
                for (i, o) in z.origins.iter().enumerate() {
                    let ok = o
                        .parse::<std::net::SocketAddr>()
                        .map(|a| std::net::TcpStream::connect_timeout(&a, Duration::from_millis(300)).is_ok())
                        .unwrap_or(false);
                    z.healthy[i].store(ok, Ordering::Relaxed);
                }
            }
        });
    }
    // stats
    {
        let sh = shared.clone();
        std::thread::spawn(move || loop {
            std::thread::sleep(Duration::from_secs(5));
            eprintln!("stats v{} waf_calls={} waf_errors={}", sh.snap.load().version, sh.waf_calls.load(Ordering::Relaxed), sh.waf_errors.load(Ordering::Relaxed));
        });
    }

    let mut server = Server::new(Some(Opt::parse_args())).unwrap();
    server.bootstrap();
    let mut svc = http_proxy_service(&server.configuration, L7(shared.clone()));
    svc.add_tcp("127.0.0.1:8080");
    let mut tls = TlsSettings::with_callbacks(Box::new(Certs(shared.clone()))).unwrap();
    tls.enable_h2();
    svc.add_tls_with_settings("127.0.0.1:8443", None, tls);
    server.add_service(svc);
    // keep the tonic runtime alive for the lifetime of the process
    std::mem::forget(rt);
    server.run_forever();
}

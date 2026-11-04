use async_trait::async_trait;
use base64::{
    Engine as _,
    engine::general_purpose::{STANDARD as BASE64, URL_SAFE_NO_PAD},
};
use reqwest::{Client, StatusCode, Url};
use secretspec_ipc::{
    error::{ErrorKind, RpcError},
    protocol::provider::{
        Address, Coordinates, InitializeApplication, Metadata, Persistence, ResolveAddressResult,
        method,
    },
    provider::{ProvidedSecret, ProviderHandler, SecretValue, serve_provider},
    server::{RequestContext, RpcResult, ServerConfig},
};
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use std::{
    env,
    fs::{self, OpenOptions},
    io::Write,
    path::{Path, PathBuf},
    sync::{
        OnceLock,
        atomic::{AtomicU64, Ordering},
    },
    time::{SystemTime, UNIX_EPOCH},
};
use zeroize::Zeroizing;

const PROVIDER: &str = "nexusq";
const ADAPTER: &str = "nexusq-secretspec";
const VERSION: &str = "1";
const ENCRYPT_METADATA: &str = "nexusq-keyprovider-secretspec-v1";
const MAX_REQUEST_BYTES: usize = 1024 * 1024;
const MAX_RESPONSE_BYTES: usize = 2 * 1024 * 1024;
static TEMP_SEQUENCE: AtomicU64 = AtomicU64::new(0);

struct Config {
    endpoint: Url,
    token: Zeroizing<String>,
    key_id: String,
    context_id: String,
    store_dir: PathBuf,
    uri: String,
    http: Client,
}

#[derive(Default)]
struct NexusQProvider {
    config: OnceLock<Config>,
}

#[derive(Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
struct StoredEnvelope {
    version: u32,
    envelope: String,
    expires_at_unix_ms: Option<u64>,
}

#[derive(Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
struct EncryptedPayload {
    adapter: String,
    version: String,
    context_id: String,
    address_binding: String,
    secret: String,
}

#[derive(Serialize)]
struct EncryptRequest<'a> {
    key_id: &'a str,
    plaintext: &'a str,
    metadata: &'static str,
}

#[derive(Serialize)]
struct DecryptRequest<'a> {
    envelope: &'a str,
}

#[derive(Deserialize)]
struct DataResponse {
    data: String,
}

fn rpc_error(kind: ErrorKind) -> RpcError {
    RpcError::new(kind)
}

fn operation_error() -> RpcError {
    rpc_error(ErrorKind::OperationFailed)
}

fn now_ms() -> Result<u64, RpcError> {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|duration| duration.as_millis() as u64)
        .map_err(|_| operation_error())
}

fn required_env(name: &str, maximum: usize) -> Result<String, RpcError> {
    let value = env::var(name).map_err(|_| operation_error())?;
    if value.is_empty() || value.len() > maximum || value.chars().any(char::is_control) {
        return Err(rpc_error(ErrorKind::InvalidParams));
    }
    Ok(value)
}

fn validate_endpoint(raw: &str) -> Result<Url, RpcError> {
    let url = Url::parse(raw).map_err(|_| rpc_error(ErrorKind::InvalidParams))?;
    if !matches!(url.scheme(), "https" | "http")
        || !url.username().is_empty()
        || url.password().is_some()
        || url.query().is_some()
        || url.fragment().is_some()
        || url.path() != "/"
        || url.port_or_known_default().is_none()
    {
        return Err(rpc_error(ErrorKind::InvalidParams));
    }
    if url.scheme() == "http" {
        let loopback = match url
            .host()
            .ok_or_else(|| rpc_error(ErrorKind::InvalidParams))?
        {
            url::Host::Ipv4(ip) => ip.is_loopback(),
            url::Host::Ipv6(ip) => ip.is_loopback(),
            url::Host::Domain(domain) => domain.eq_ignore_ascii_case("localhost"),
        };
        if !loopback {
            return Err(rpc_error(ErrorKind::InvalidParams));
        }
    }
    Ok(url)
}

fn validate_provider_uri(raw: &str) -> Result<(), RpcError> {
    let uri = Url::parse(raw).map_err(|_| rpc_error(ErrorKind::InvalidParams))?;
    if uri.scheme() != PROVIDER
        || uri.host().is_none()
        || !uri.username().is_empty()
        || uri.password().is_some()
        || uri.query().is_some()
        || uri.fragment().is_some()
        || !matches!(uri.path(), "" | "/")
        || uri.port().is_some()
    {
        return Err(rpc_error(ErrorKind::InvalidParams));
    }
    Ok(())
}

fn default_store_dir() -> Result<PathBuf, RpcError> {
    if let Some(path) = env::var_os("NEXUSQ_STORE_DIR") {
        return Ok(PathBuf::from(path));
    }
    if let Some(path) = env::var_os("XDG_DATA_HOME") {
        return Ok(PathBuf::from(path).join("nexusq-secretspec"));
    }
    let home = env::var_os("HOME").ok_or_else(operation_error)?;
    Ok(PathBuf::from(home).join(".local/share/nexusq-secretspec"))
}

fn secure_store_dir(path: &Path) -> Result<PathBuf, RpcError> {
    fs::create_dir_all(path).map_err(|_| operation_error())?;
    let metadata = fs::symlink_metadata(path).map_err(|_| operation_error())?;
    if metadata.file_type().is_symlink() || !metadata.is_dir() {
        return Err(operation_error());
    }
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        fs::set_permissions(path, fs::Permissions::from_mode(0o700))
            .map_err(|_| operation_error())?;
    }
    fs::canonicalize(path).map_err(|_| operation_error())
}

fn canonical_address(address: &Address) -> Result<Address, RpcError> {
    match address {
        Address::Convention {
            project,
            profile,
            key,
        } => {
            // Encode the tuple rather than joining with a delimiter: project,
            // profile, and key may themselves contain delimiter characters.
            if project.is_empty() || profile.is_empty() || key.is_empty() {
                return Err(rpc_error(ErrorKind::InvalidParams));
            }
            let tuple =
                serde_json::to_vec(&[project, profile, key]).map_err(|_| operation_error())?;
            let item = format!("nexusq-v1:{}", URL_SAFE_NO_PAD.encode(tuple));
            if item.len() > 4096 {
                return Err(rpc_error(ErrorKind::InvalidParams));
            }
            Ok(Address::Native {
                coordinates: Coordinates {
                    item,
                    field: None,
                    vault: None,
                    section: None,
                    version: None,
                },
            })
        }
        Address::Native { coordinates } => Ok(Address::Native {
            coordinates: coordinates.clone(),
        }),
    }
}

fn address_binding(config: &Config, address: &Address) -> Result<String, RpcError> {
    let canonical = canonical_address(address)?;
    let bytes = serde_json::to_vec(&canonical).map_err(|_| operation_error())?;
    let mut hasher = Sha256::new();
    hasher.update(config.uri.as_bytes());
    hasher.update([0]);
    hasher.update(bytes);
    Ok(hex::encode(hasher.finalize()))
}

fn file_path(config: &Config, address: &Address) -> Result<PathBuf, RpcError> {
    let canonical = canonical_address(address)?;
    let mut hasher = Sha256::new();
    hasher.update(config.uri.as_bytes());
    hasher.update([0]);
    hasher.update(serde_json::to_vec(&canonical).map_err(|_| operation_error())?);
    let name = hex::encode(hasher.finalize());
    Ok(config.store_dir.join(format!("{name}.json")))
}

fn read_record(path: &Path) -> Result<Option<StoredEnvelope>, RpcError> {
    let metadata = match fs::symlink_metadata(path) {
        Ok(metadata) => metadata,
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => return Ok(None),
        Err(_) => return Err(operation_error()),
    };
    if metadata.file_type().is_symlink() || !metadata.is_file() {
        return Err(operation_error());
    }
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        if metadata.permissions().mode() & 0o077 != 0 {
            return Err(operation_error());
        }
    }
    if metadata.len() > MAX_REQUEST_BYTES as u64 {
        return Err(operation_error());
    }
    let bytes = fs::read(path).map_err(|_| operation_error())?;
    let record: StoredEnvelope = serde_json::from_slice(&bytes).map_err(|_| operation_error())?;
    if record.version != 1 || record.envelope.is_empty() {
        return Err(operation_error());
    }
    Ok(Some(record))
}

fn write_record(path: &Path, record: &StoredEnvelope) -> Result<(), RpcError> {
    let bytes = serde_json::to_vec(record).map_err(|_| operation_error())?;
    if bytes.len() > MAX_REQUEST_BYTES {
        return Err(operation_error());
    }
    let sequence = TEMP_SEQUENCE.fetch_add(1, Ordering::Relaxed);
    let temp = path.with_extension(format!("{}.{}.tmp", std::process::id(), sequence));
    let mut options = OpenOptions::new();
    options.write(true).create_new(true);
    #[cfg(unix)]
    {
        use std::os::unix::fs::OpenOptionsExt;
        options.mode(0o600);
    }
    let mut file = options.open(&temp).map_err(|_| operation_error())?;
    let result = (|| {
        file.write_all(&bytes).map_err(|_| operation_error())?;
        file.sync_all().map_err(|_| operation_error())?;
        fs::rename(&temp, path).map_err(|_| operation_error())?;
        #[cfg(unix)]
        if let Some(parent) = path.parent() {
            OpenOptions::new()
                .read(true)
                .open(parent)
                .and_then(|directory| directory.sync_all())
                .map_err(|_| operation_error())?;
        }
        Ok::<(), RpcError>(())
    })();
    if result.is_err() {
        let _ = fs::remove_file(&temp);
    }
    result
}

impl Config {
    fn from_environment(uri: String) -> Result<Self, RpcError> {
        validate_provider_uri(&uri)?;
        let raw_endpoint = required_env("NEXUSQ_URL", 2048)?;
        let endpoint = validate_endpoint(&raw_endpoint)?;
        let token = Zeroizing::new(required_env("NEXUSQ_TOKEN", 8192)?);
        if token.len() < 32 || token.chars().any(char::is_whitespace) {
            return Err(rpc_error(ErrorKind::InvalidParams));
        }
        let key_id = required_env("NEXUSQ_KEY_ID", 256)?;
        let context_id = required_env("NEXUSQ_CONTEXT_ID", 256)?;
        let store_dir = secure_store_dir(&default_store_dir()?)?;
        let mut http_builder = Client::builder()
            .redirect(reqwest::redirect::Policy::none())
            .connect_timeout(std::time::Duration::from_secs(5))
            .timeout(std::time::Duration::from_secs(30));
        // Do not let an ambient proxy intercept local development traffic or
        // receive the bearer token intended for a loopback NEXUS-Q instance.
        if endpoint.scheme() == "http" {
            http_builder = http_builder.no_proxy();
        }
        let http = http_builder.build().map_err(|_| operation_error())?;
        Ok(Self {
            endpoint,
            token,
            key_id,
            context_id,
            store_dir,
            uri,
            http,
        })
    }

    async fn post<T: Serialize>(&self, path: &str, body: &T) -> Result<Vec<u8>, RpcError> {
        let url = self.endpoint.join(path).map_err(|_| operation_error())?;
        let body = serde_json::to_vec(body).map_err(|_| operation_error())?;
        if body.len() > MAX_REQUEST_BYTES {
            return Err(rpc_error(ErrorKind::InvalidParams));
        }
        let response = self
            .http
            .post(url)
            .bearer_auth(self.token.as_str())
            .header(reqwest::header::CONTENT_TYPE, "application/json")
            .body(body)
            .send()
            .await
            .map_err(|_| operation_error())?;
        let status = response.status();
        if status == StatusCode::UNAUTHORIZED
            || status == StatusCode::FORBIDDEN
            || !status.is_success()
        {
            return Err(operation_error());
        }
        if response
            .content_length()
            .is_some_and(|length| length > MAX_RESPONSE_BYTES as u64)
        {
            return Err(operation_error());
        }
        let mut response = response;
        let mut bytes = Vec::new();
        while let Some(chunk) = response.chunk().await.map_err(|_| operation_error())? {
            if bytes.len().saturating_add(chunk.len()) > MAX_RESPONSE_BYTES {
                return Err(operation_error());
            }
            bytes.extend_from_slice(&chunk);
        }
        Ok(bytes)
    }

    async fn encrypt(&self, address: &Address, secret: &str) -> Result<String, RpcError> {
        if secret.len() > MAX_REQUEST_BYTES {
            return Err(rpc_error(ErrorKind::InvalidParams));
        }
        let payload = EncryptedPayload {
            adapter: ADAPTER.to_string(),
            version: VERSION.to_string(),
            context_id: self.context_id.clone(),
            address_binding: address_binding(self, address)?,
            secret: secret.to_string(),
        };
        let plaintext = serde_json::to_string(&payload).map_err(|_| operation_error())?;
        let response = self
            .post(
                "/v1/encrypt",
                &EncryptRequest {
                    key_id: &self.key_id,
                    plaintext: &plaintext,
                    metadata: ENCRYPT_METADATA,
                },
            )
            .await?;
        let decoded: DataResponse =
            serde_json::from_slice(&response).map_err(|_| operation_error())?;
        let envelope = BASE64
            .decode(&decoded.data)
            .map_err(|_| operation_error())?;
        if envelope.is_empty()
            || decoded.data.len() + 32 > MAX_REQUEST_BYTES
            || BASE64.encode(&envelope) != decoded.data
        {
            return Err(operation_error());
        }
        Ok(decoded.data)
    }

    async fn decrypt(&self, address: &Address, envelope: &str) -> Result<String, RpcError> {
        if envelope.is_empty()
            || envelope.len() + 32 > MAX_REQUEST_BYTES
            || BASE64
                .decode(envelope)
                .map(|decoded| decoded.is_empty() || BASE64.encode(decoded) != envelope)
                .unwrap_or(true)
        {
            return Err(operation_error());
        }
        let request = DecryptRequest { envelope };
        let response = self.post("/v1/decrypt", &request).await?;
        let decoded: DataResponse =
            serde_json::from_slice(&response).map_err(|_| operation_error())?;
        let plaintext = BASE64
            .decode(&decoded.data)
            .map_err(|_| operation_error())?;
        if BASE64.encode(&plaintext) != decoded.data {
            return Err(operation_error());
        }
        let payload: EncryptedPayload =
            serde_json::from_slice(&plaintext).map_err(|_| operation_error())?;
        if payload.adapter != ADAPTER
            || payload.version != VERSION
            || payload.context_id != self.context_id
            || payload.address_binding != address_binding(self, address)?
        {
            return Err(operation_error());
        }
        Ok(payload.secret)
    }

    async fn store(
        &self,
        address: &Address,
        value: &str,
        expiry: Option<u64>,
    ) -> Result<(), RpcError> {
        let envelope = self.encrypt(address, value).await?;
        let record = StoredEnvelope {
            version: 1,
            envelope,
            expires_at_unix_ms: expiry,
        };
        write_record(&file_path(self, address)?, &record)
    }

    async fn load(&self, address: &Address) -> Result<Option<ProvidedSecret>, RpcError> {
        let path = file_path(self, address)?;
        let Some(record) = read_record(&path)? else {
            return Ok(None);
        };
        if let Some(expiry) = record.expires_at_unix_ms
            && expiry <= now_ms()?
        {
            let _ = fs::remove_file(path);
            return Ok(None);
        }
        let value = self.decrypt(address, &record.envelope).await?;
        Ok(Some(ProvidedSecret::new(value, record.expires_at_unix_ms)))
    }
}

#[async_trait]
impl ProviderHandler for NexusQProvider {
    fn capabilities(&self) -> Vec<String> {
        vec![
            method::RESOLVE_ADDRESS.to_string(),
            method::GET.to_string(),
            method::EXISTS.to_string(),
            method::SET.to_string(),
            method::SET_EXPIRING.to_string(),
            method::DELETE.to_string(),
            method::CHECK_WRITABLE.to_string(),
            method::CHECK_DELETABLE.to_string(),
            method::DESCRIBE_WRITE_TARGET.to_string(),
        ]
    }

    async fn initialize(
        &self,
        _context: &RequestContext,
        application: InitializeApplication,
    ) -> RpcResult<Metadata> {
        if application.scheme != PROVIDER {
            return Err(rpc_error(ErrorKind::InvalidParams));
        }
        let config = Config::from_environment(application.uri.clone())?;
        let path = config.store_dir.display().to_string();
        self.config
            .set(config)
            .map_err(|_| rpc_error(ErrorKind::Conflict))?;
        Ok(Metadata {
            name: PROVIDER.to_string(),
            display_uri: application.uri,
            supported_coordinates: vec![],
            generated_value_persistence: Persistence::Persist,
            prompted_value_persistence: Persistence::Persist,
            storage_identity: "NEXUS-Q encrypted local envelope store".to_string(),
            entry_container_identity: "one encrypted envelope per SecretSpec address".to_string(),
            physical_store_path: Some(path),
        })
    }

    async fn resolve_address(
        &self,
        _context: RequestContext,
        address: Address,
    ) -> RpcResult<ResolveAddressResult> {
        let Address::Native { coordinates } = canonical_address(&address)? else {
            unreachable!("canonical addresses are native")
        };
        Ok(ResolveAddressResult { coordinates })
    }

    async fn get(
        &self,
        _context: RequestContext,
        address: Address,
    ) -> RpcResult<Option<ProvidedSecret>> {
        self.config
            .get()
            .ok_or_else(operation_error)?
            .load(&address)
            .await
    }

    async fn exists(&self, _context: RequestContext, address: Address) -> RpcResult<bool> {
        let config = self.config.get().ok_or_else(operation_error)?;
        let Some(record) = read_record(&file_path(config, &address)?)? else {
            return Ok(false);
        };
        if let Some(expiry) = record.expires_at_unix_ms
            && expiry <= now_ms()?
        {
            let _ = fs::remove_file(file_path(config, &address)?);
            return Ok(false);
        }
        Ok(true)
    }

    async fn set(
        &self,
        _context: RequestContext,
        address: Address,
        value: SecretValue,
    ) -> RpcResult<()> {
        self.config
            .get()
            .ok_or_else(operation_error)?
            .store(&address, value.expose(), None)
            .await
    }

    async fn set_expiring(
        &self,
        _context: RequestContext,
        address: Address,
        value: SecretValue,
        ttl_ms: u64,
    ) -> RpcResult<()> {
        let expiry = now_ms()?.checked_add(ttl_ms).ok_or_else(operation_error)?;
        self.config
            .get()
            .ok_or_else(operation_error)?
            .store(&address, value.expose(), Some(expiry))
            .await
    }

    async fn delete(&self, _context: RequestContext, address: Address) -> RpcResult<bool> {
        let path = file_path(self.config.get().ok_or_else(operation_error)?, &address)?;
        match fs::remove_file(path) {
            Ok(()) => Ok(true),
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => Ok(false),
            Err(_) => Err(operation_error()),
        }
    }

    async fn check_writable(&self, _context: RequestContext, _address: Address) -> RpcResult<()> {
        Ok(())
    }

    async fn check_deletable(&self, _context: RequestContext, _address: Address) -> RpcResult<()> {
        Ok(())
    }

    async fn describe_write_target(
        &self,
        _context: RequestContext,
        _address: Address,
    ) -> RpcResult<String> {
        let config = self.config.get().ok_or_else(operation_error)?;
        Ok(format!(
            "NEXUS-Q encrypted envelope in {}",
            config.store_dir.display()
        ))
    }
}

#[tokio::main]
async fn main() {
    let handler = NexusQProvider::default();
    if let Err(_error) = serve_provider(
        tokio::io::stdin(),
        tokio::io::stdout(),
        handler,
        ServerConfig {
            product: secretspec_ipc::Product {
                name: "nexusq-secretspec-provider".to_string(),
                version: env!("CARGO_PKG_VERSION").to_string(),
            },
            ..ServerConfig::default()
        },
    )
    .await
    {
        eprintln!("secretspec-provider-nexusq: provider session failed");
        std::process::exit(1);
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;

    fn convention(project: &str, profile: &str, key: &str) -> Address {
        Address::Convention {
            project: project.to_string(),
            profile: profile.to_string(),
            key: key.to_string(),
        }
    }

    #[test]
    fn only_https_and_loopback_http_are_accepted() {
        assert!(validate_endpoint("https://nexusq.example").is_ok());
        assert!(validate_endpoint("http://127.0.0.1:8080").is_ok());
        assert!(validate_endpoint("http://[::1]:8080").is_ok());
        assert!(validate_endpoint("http://localhost:8080").is_ok());
        assert!(validate_endpoint("http://nexusq.example").is_err());
        assert!(validate_endpoint("https://user:pass@nexusq.example").is_err());
        assert!(validate_endpoint("https://nexusq.example/api").is_err());
        assert!(validate_endpoint("https://nexusq.example/?token=x").is_err());
        assert!(validate_endpoint("https://nexusq.example/#fragment").is_err());
        assert!(validate_provider_uri("nexusq://default").is_ok());
        assert!(validate_provider_uri("nexusq://user:secret@default").is_err());
        assert!(validate_provider_uri("nexusq://default?token=secret").is_err());
        assert!(validate_provider_uri("nexusq://default/path").is_err());
    }

    #[test]
    fn address_binding_changes_between_addresses_and_provider_uris() {
        let address = convention("payments", "production", "DATABASE_URL");
        let other = convention("payments", "staging", "DATABASE_URL");
        let resolved = canonical_address(&address).unwrap();
        let config = test_config("nexusq://default");
        assert_eq!(
            address_binding(&config, &address).unwrap(),
            address_binding(&config, &resolved).unwrap()
        );
        assert_eq!(
            file_path(&config, &address).unwrap(),
            file_path(&config, &resolved).unwrap()
        );
        let other_provider = test_config("nexusq://secondary");
        assert_ne!(
            address_binding(&config, &address).unwrap(),
            address_binding(&config, &other).unwrap()
        );
        assert_ne!(
            address_binding(&config, &address).unwrap(),
            address_binding(&other_provider, &address).unwrap()
        );
    }

    #[test]
    fn local_record_contains_ciphertext_only_and_is_private() {
        let dir = tempdir().unwrap();
        let root = secure_store_dir(dir.path().join("store").as_path()).unwrap();
        let path = root.join("record.json");
        let record = StoredEnvelope {
            version: 1,
            envelope: "Y2lwaGVydGV4dC1vbmx5".to_string(),
            expires_at_unix_ms: None,
        };
        write_record(&path, &record).unwrap();
        let bytes = fs::read(&path).unwrap();
        assert!(!String::from_utf8_lossy(&bytes).contains("plaintext-secret"));
        assert_eq!(
            read_record(&path).unwrap().unwrap().envelope,
            record.envelope
        );
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            assert_eq!(
                fs::metadata(&path).unwrap().permissions().mode() & 0o777,
                0o600
            );
            assert_eq!(
                fs::metadata(&root).unwrap().permissions().mode() & 0o777,
                0o700
            );
        }
    }

    fn test_config(uri: &str) -> Config {
        Config {
            endpoint: Url::parse("https://nexusq.example").unwrap(),
            token: Zeroizing::new("x".repeat(32)),
            key_id: "test-key".into(),
            context_id: "test-context".into(),
            store_dir: PathBuf::from("/tmp/nexusq-secretspec-test"),
            uri: uri.into(),
            http: Client::new(),
        }
    }
}

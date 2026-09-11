//! Rolle core. Owns sessions, provider abstraction, and credential storage.
//! The daemon, CLI, and desktop app all depend on this crate.

use serde::{Deserialize, Serialize};

/// Short-lived credentials for one cloud session.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Credentials {
    pub access_key_id: String,
    pub secret_access_key: String,
    pub session_token: Option<String>,
    /// RFC 3339 expiry time.
    pub expiration: Option<String>,
}

/// One configured way to obtain credentials.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Session {
    pub id: String,
    pub name: String,
    pub region: String,
}

/// A credential source. Each cloud or auth flow implements this trait.
pub trait Provider {
    /// Return the provider kind, for example `aws-sso`.
    fn kind(&self) -> &'static str;
    /// Produce fresh credentials for `session`.
    fn credentials(&self, session: &Session) -> anyhow::Result<Credentials>;
}

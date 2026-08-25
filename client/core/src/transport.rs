//! HTTP transport abstraction. The core builds `HttpRequest`s and interprets
//! `HttpResponse`s; the actual bytes-on-the-wire live behind the `Transport`
//! trait so the client logic is testable with a fake and the real network layer
//! (feature `net`) is swappable per platform.

use crate::error::Result;
#[cfg(feature = "net")]
use crate::error::CoreError;

#[derive(Debug, Clone)]
pub struct HttpRequest {
    pub method: String,
    pub url: String,
    pub bearer: Option<String>,
    pub body: Option<String>,
}

#[derive(Debug, Clone)]
pub struct HttpResponse {
    pub status: u16,
    pub body: String,
}

impl HttpResponse {
    pub fn is_success(&self) -> bool {
        (200..300).contains(&self.status)
    }
}

/// A transport sends an `HttpRequest` and returns an `HttpResponse`. It returns
/// `Err` only for network-level failures; an HTTP error status is a successful
/// send with a non-2xx `HttpResponse`.
pub trait Transport: Send + Sync {
    fn send(&self, req: HttpRequest) -> Result<HttpResponse>;
}

/// Real HTTP transport backed by ureq (blocking, TLS via rustls). Enabled with
/// the `net` feature so tests need not compile a TLS stack.
#[cfg(feature = "net")]
pub struct UreqTransport;

#[cfg(feature = "net")]
impl Transport for UreqTransport {
    fn send(&self, req: HttpRequest) -> Result<HttpResponse> {
        let mut builder = ureq::request(&req.method, &req.url);
        if let Some(token) = &req.bearer {
            builder = builder.set("Authorization", &format!("Bearer {token}"));
        }
        let result = match req.body {
            Some(body) => builder.set("Content-Type", "application/json").send_string(&body),
            None => builder.call(),
        };
        match result {
            Ok(resp) => {
                let status = resp.status();
                let body = resp
                    .into_string()
                    .map_err(|e| CoreError::Transport(e.to_string()))?;
                Ok(HttpResponse { status, body })
            }
            // A non-2xx status is a response, not a transport failure.
            Err(ureq::Error::Status(code, resp)) => Ok(HttpResponse {
                status: code,
                body: resp.into_string().unwrap_or_default(),
            }),
            Err(e) => Err(CoreError::Transport(e.to_string())),
        }
    }
}

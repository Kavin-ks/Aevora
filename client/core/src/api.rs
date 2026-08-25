//! Typed client for the Aevora control-plane API, over a `Transport`.

use serde::de::DeserializeOwned;
use serde::Deserialize;

use crate::error::{CoreError, Result};
use crate::model::*;
use crate::transport::{HttpRequest, HttpResponse, Transport};

pub struct ApiClient {
    base_url: String,
    transport: Box<dyn Transport>,
}

#[derive(Deserialize)]
struct ApiErrorBody {
    error: String,
}

impl ApiClient {
    pub fn new(base_url: impl Into<String>, transport: Box<dyn Transport>) -> Self {
        let mut base = base_url.into();
        while base.ends_with('/') {
            base.pop();
        }
        Self { base_url: base, transport }
    }

    fn call(&self, method: &str, path: &str, bearer: Option<&str>, body: Option<String>) -> Result<HttpResponse> {
        self.transport.send(HttpRequest {
            method: method.to_string(),
            url: format!("{}{}", self.base_url, path),
            bearer: bearer.map(str::to_string),
            body,
        })
    }

    /// Enroll with an invite; returns access + refresh tokens.
    pub fn enroll(&self, req: &EnrollRequest) -> Result<TokenResponse> {
        let resp = self.call("POST", "/v1/enroll", None, Some(to_json(req)?))?;
        decode(resp)
    }

    /// Exchange a refresh token for a new access token.
    pub fn refresh(&self, refresh_token: &str) -> Result<TokenResponse> {
        let body = format!(r#"{{"refresh_token":{}}}"#, json_string(refresh_token));
        let resp = self.call("POST", "/v1/auth/refresh", None, Some(body))?;
        decode(resp)
    }

    /// List selectable countries.
    pub fn locations(&self, access: &str) -> Result<Vec<Location>> {
        let resp = self.call("GET", "/v1/locations", Some(access), None)?;
        let out: LocationsResponse = decode(resp)?;
        Ok(out.locations)
    }

    /// Request a connection (select gateway + lease address).
    pub fn connect(&self, access: &str, req: &ConnectRequest) -> Result<ConnectionResponse> {
        let resp = self.call("POST", "/v1/connections", Some(access), Some(to_json(req)?))?;
        decode(resp)
    }

    /// Release a connection.
    pub fn disconnect(&self, access: &str, connection_id: &str) -> Result<()> {
        let resp = self.call("DELETE", &format!("/v1/connections/{connection_id}"), Some(access), None)?;
        expect_success(resp)
    }

    /// Report stats / keep the lease alive.
    pub fn stats(&self, access: &str, connection_id: &str, report: &StatsReport) -> Result<()> {
        let resp = self.call(
            "POST",
            &format!("/v1/connections/{connection_id}/stats"),
            Some(access),
            Some(to_json(report)?),
        )?;
        expect_success(resp)
    }
}

fn to_json<T: serde::Serialize>(v: &T) -> Result<String> {
    serde_json::to_string(v).map_err(|e| CoreError::Decode(e.to_string()))
}

/// JSON-encodes a bare string (with quotes and escaping).
fn json_string(s: &str) -> String {
    serde_json::to_string(s).unwrap_or_else(|_| "\"\"".to_string())
}

fn decode<T: DeserializeOwned>(resp: HttpResponse) -> Result<T> {
    if !resp.is_success() {
        return Err(api_error(resp));
    }
    serde_json::from_str(&resp.body).map_err(|e| CoreError::Decode(e.to_string()))
}

fn expect_success(resp: HttpResponse) -> Result<()> {
    if resp.is_success() {
        Ok(())
    } else {
        Err(api_error(resp))
    }
}

fn api_error(resp: HttpResponse) -> CoreError {
    let message = serde_json::from_str::<ApiErrorBody>(&resp.body)
        .map(|b| b.error)
        .unwrap_or_else(|_| resp.body.clone());
    CoreError::Api { status: resp.status, message }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::{Arc, Mutex};

    type Recorder = Arc<Mutex<Option<HttpRequest>>>;

    struct FakeTransport {
        response: HttpResponse,
        last: Recorder,
    }
    impl FakeTransport {
        fn ok(body: &str) -> Self {
            Self { response: HttpResponse { status: 200, body: body.into() }, last: Arc::new(Mutex::new(None)) }
        }
        fn status(status: u16, body: &str) -> Self {
            Self { response: HttpResponse { status, body: body.into() }, last: Arc::new(Mutex::new(None)) }
        }
    }
    impl Transport for FakeTransport {
        fn send(&self, req: HttpRequest) -> Result<HttpResponse> {
            *self.last.lock().unwrap() = Some(req);
            Ok(self.response.clone())
        }
    }

    #[test]
    fn enroll_posts_and_parses_tokens() {
        let ft = Box::new(FakeTransport::ok(
            r#"{"access_token":"AT","token_type":"Bearer","expires_in":900,"refresh_token":"RT","user_id":"u","device_id":"d"}"#,
        ));
        let api = ApiClient::new("http://x/", ft);
        let req = EnrollRequest {
            invite_code: "inv".into(),
            email: "a@b.co".into(),
            display_name: None,
            device: DeviceRegistration { name: "l".into(), platform: "macos".into(), public_key: "PUB=".into() },
        };
        let tok = api.enroll(&req).unwrap();
        assert_eq!(tok.access_token, "AT");
        assert_eq!(tok.refresh_token, "RT");
        assert_eq!(tok.device_id, "d");
    }

    #[test]
    fn locations_sends_bearer_and_strips_wrapper() {
        let ft = FakeTransport::ok(r#"{"locations":[{"code":"de","country":"Germany","available":true,"servers":2}]}"#);
        let recorded = ft.last.clone(); // clone the Arc before moving ft
        let api = ApiClient::new("http://x", Box::new(ft));
        let locs = api.locations("ACCESS").unwrap();
        assert_eq!(locs.len(), 1);
        assert_eq!(locs[0].code, "de");
        let req = recorded.lock().unwrap().take().unwrap();
        assert_eq!(req.method, "GET");
        assert_eq!(req.url, "http://x/v1/locations");
        assert_eq!(req.bearer.as_deref(), Some("ACCESS"));
    }

    #[test]
    fn non_2xx_becomes_api_error_with_message() {
        let ft = Box::new(FakeTransport::status(403, r#"{"error":"invite is invalid"}"#));
        let api = ApiClient::new("http://x", ft);
        let req = EnrollRequest {
            invite_code: "bad".into(),
            email: "a@b.co".into(),
            display_name: None,
            device: DeviceRegistration { name: "l".into(), platform: "ios".into(), public_key: "P".into() },
        };
        match api.enroll(&req) {
            Err(CoreError::Api { status, message }) => {
                assert_eq!(status, 403);
                assert_eq!(message, "invite is invalid");
            }
            other => panic!("expected Api error, got {other:?}"),
        }
    }

    #[test]
    fn disconnect_checks_status_only() {
        let ft = Box::new(FakeTransport::status(204, ""));
        let api = ApiClient::new("http://x", ft);
        assert!(api.disconnect("ACCESS", "lease-1").is_ok());
    }
}

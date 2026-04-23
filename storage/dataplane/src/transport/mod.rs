//! Transport layer.
//!
//! On single-node NovaNas the "inter-node" concept is vestigial: there is no
//! peer dataplane to dial. What remains here is:
//!   * `chunk_service` / `dataplane_service` — gRPC servers exposed to the Go
//!     control plane and the CSI agent.
//!   * `ndp_server` — local NDP listener for sub-block I/O.
//!   * `server` — bootstrap wiring for the above.
//!
//! The previous `chunk_client` (gRPC client that dialed peer dataplanes) has
//! been removed as dead code — see docs/14 S9.

pub mod chunk_service;
pub mod dataplane_service;
pub mod ndp_server;
pub mod server;

pub mod chunk_proto {
    tonic::include_proto!("chunk");
}

pub mod dataplane_proto {
    tonic::include_proto!("dataplane");
}

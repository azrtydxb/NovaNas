//! Build script: compile the canonical MetaService gRPC contract from
//! `storage/api/proto/meta/meta.proto`. The frontend only needs the
//! *client* side in production, but the integration tests at
//! `tests/meta_client.rs` stand up an in-process tonic server, so we keep
//! both client and server stubs available.

fn main() {
    let proto = "../api/proto/meta/meta.proto";
    let include_root = "../api/proto/";

    tonic_build::configure()
        .build_client(true)
        .build_server(true)
        .compile_protos(&[proto], &[include_root])
        .expect("failed to compile meta.proto");

    println!("cargo:rerun-if-changed={proto}");
    println!("cargo:rerun-if-changed=build.rs");
}

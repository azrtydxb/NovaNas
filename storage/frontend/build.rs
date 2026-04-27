//! Build script: compile the meta + chunk gRPC contracts so the frontend
//! can call meta and (eventually) the data daemon's chunk control RPCs.
//!
//! The frontend only needs the *client* side — it never serves these
//! services. Server stubs are still emitted because tonic-build defaults
//! to both; suppressing them would just complicate the build.

fn main() {
    // Build both client and server stubs. The frontend only ever
    // *calls* meta and chunk in production, but the integration tests
    // (`tests/meta_client.rs`) stand up an in-process tonic server, so
    // we keep server stubs available too.
    tonic_build::configure()
        .build_client(true)
        .build_server(true)
        .compile_protos(&["proto/meta.proto", "proto/chunk.proto"], &["proto/"])
        .expect("failed to compile proto files");

    println!("cargo:rerun-if-changed=proto/meta.proto");
    println!("cargo:rerun-if-changed=proto/chunk.proto");
    println!("cargo:rerun-if-changed=build.rs");
}

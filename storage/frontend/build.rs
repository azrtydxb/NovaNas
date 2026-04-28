//! Build script for novanas-frontend.
//!
//! Always: compile `MetaService` gRPC stubs (the integration test stands
//! up an in-process tonic server, so we need both client and server).
//!
//! When `spdk-sys` is on: emit the same SPDK / DPDK / ISA-L link config
//! as `storage/dataplane/build.rs`, compile the `uring_wrapper.c` shim,
//! and run bindgen against `src/spdk_wrapper.h`. Without `spdk-sys` the
//! script writes a stub `spdk_bindings.rs` so any `include!()` against
//! the bindings file resolves to an empty module.

use std::env;
use std::path::PathBuf;

fn main() {
    // Compile MetaService — needed by both client (production) and
    // server (tests/meta_client.rs).
    let proto = "../api/proto/meta/meta.proto";
    let include_root = "../api/proto/";
    tonic_build::configure()
        .build_client(true)
        .build_server(true)
        .compile_protos(&[proto], &[include_root])
        .expect("failed to compile meta.proto");
    println!("cargo:rerun-if-changed={proto}");
    println!("cargo:rerun-if-changed=build.rs");

    // Always emit a stub spdk_bindings.rs unless the spdk-sys feature is
    // enabled — this keeps non-SPDK `cargo test` runs working without a
    // SPDK install.
    if env::var("CARGO_FEATURE_SPDK_SYS").is_err() {
        let out_path = PathBuf::from(env::var("OUT_DIR").unwrap());
        std::fs::write(
            out_path.join("spdk_bindings.rs"),
            b"// stub: spdk-sys feature not enabled\n",
        )
        .expect("write stub bindings");
        return;
    }

    // ---- SPDK linkage (mirrors storage/dataplane/build.rs) ----

    let spdk_dir = env::var("SPDK_DIR").unwrap_or_else(|_| "/usr/local".to_string());
    println!("cargo:rustc-link-search=native={}/lib", spdk_dir);
    println!("cargo:rerun-if-env-changed=SPDK_DIR");

    let lib_dir_path = format!("{}/lib", spdk_dir);

    let mut static_libs: Vec<String> = Vec::new();
    if let Ok(entries) = std::fs::read_dir(&lib_dir_path) {
        for entry in entries.flatten() {
            let name = entry.file_name().to_string_lossy().to_string();
            if (name.starts_with("libspdk_")
                || name.starts_with("librte_")
                || name.starts_with("libisal"))
                && name.ends_with(".a")
                // iSCSI is unused by the frontend (NVMe-oF only) and its PDU
                // pool wastes ~300MB of hugepage memory at init.
                && !matches!(name.as_str(), "libspdk_iscsi.a" | "libspdk_event_iscsi.a")
            {
                static_libs.push(entry.path().to_string_lossy().to_string());
            }
        }
    }
    static_libs.sort();

    println!("cargo:rustc-link-arg=-Wl,--whole-archive");
    println!("cargo:rustc-link-arg=-Wl,--start-group");
    for lib_path in &static_libs {
        println!("cargo:rustc-link-arg={}", lib_path);
    }
    println!("cargo:rustc-link-arg=-Wl,--end-group");
    println!("cargo:rustc-link-arg=-Wl,--no-whole-archive");

    for lib in &[
        "c", "m", "dl", "rt", "pthread", "aio", "uuid", "numa", "ssl", "crypto", "uring", "fuse3",
        "json-c", "gcc_s", "stdc++",
    ] {
        println!("cargo:rustc-link-arg=-l{}", lib);
    }

    println!("cargo:rustc-link-search=native=/usr/lib/aarch64-linux-gnu");
    println!("cargo:rustc-link-search=native=/usr/lib/x86_64-linux-gnu");
    for ver in &["12", "13", "14"] {
        println!(
            "cargo:rustc-link-search=native=/usr/lib/gcc/aarch64-linux-gnu/{}",
            ver
        );
        println!(
            "cargo:rustc-link-search=native=/usr/lib/gcc/x86_64-linux-gnu/{}",
            ver
        );
    }

    let spdk_include = format!("{}/include", spdk_dir);
    cc::Build::new()
        .file("src/uring_wrapper.c")
        .include(&spdk_include)
        .compile("uring_wrapper");

    if PathBuf::from(&spdk_include).exists() {
        let bindings = bindgen::Builder::default()
            .header("src/spdk_wrapper.h")
            .clang_arg(format!("-I{}", spdk_include))
            .allowlist_function("spdk_app_.*")
            .allowlist_function("spdk_bdev_.*")
            .allowlist_function("spdk_nvmf_.*")
            .allowlist_function("spdk_nvme_transport_id.*")
            .allowlist_function("spdk_json.*")
            .allowlist_function("spdk_rpc.*")
            .allowlist_function("spdk_log.*")
            .allowlist_function("spdk_thread.*")
            .allowlist_function("spdk_dma_.*")
            .allowlist_function("spdk_io_device_register")
            .allowlist_function("spdk_io_device_unregister")
            .allowlist_function("spdk_get_io_channel")
            .allowlist_function("spdk_put_io_channel")
            .allowlist_function("spdk_iobuf_set_opts")
            .allowlist_function("spdk_iobuf_get_opts")
            .allowlist_type("spdk_iobuf_opts")
            .allowlist_function("create_malloc_disk")
            .allowlist_function("delete_malloc_disk")
            .allowlist_function("novanas_create_uring_bdev")
            .allowlist_type("malloc_bdev_opts")
            .allowlist_type("spdk_app_opts")
            .allowlist_type("spdk_bdev")
            .allowlist_type("spdk_bdev_io")
            .allowlist_type("spdk_bdev_desc")
            .allowlist_function("spdk_bdev_desc_get_bdev")
            .allowlist_function("spdk_bdev_get_name")
            .allowlist_function("spdk_bdev_first")
            .allowlist_function("spdk_bdev_next")
            .allowlist_type("spdk_nvmf_target")
            .allowlist_type("spdk_nvmf_subsystem")
            .allowlist_type("spdk_nvmf_transport_id")
            .allowlist_var("SPDK_.*")
            .layout_tests(false)
            .derive_default(true)
            .derive_copy(false)
            .disable_header_comment()
            .raw_line("#[allow(non_camel_case_types, non_snake_case, non_upper_case_globals, dead_code, improper_ctypes)]")
            .raw_line("")
            .opaque_type("spdk_nvme_ctrlr_data")
            .opaque_type("spdk_nvmf_fabric_connect_rsp")
            .opaque_type("spdk_nvmf_fabric_prop_get_rsp")
            .opaque_type("spdk_bdev_ext_io_opts")
            .opaque_type("spdk_nvme_tcp_cmd")
            .opaque_type("spdk_nvme_tcp_rsp")
            .opaque_type("spdk_nvmf_transport_opts")
            .opaque_type("spdk_nvmf_ctrlr_feat")
            .opaque_type("spdk_nvmf_ctrlr_migr_data")
            .opaque_type("nvmf_h2c_msg")
            .opaque_type("spdk_nvme_cmd")
            .opaque_type("spdk_bdev_io_nvme_passthru_params")
            .generate()
            .expect("Unable to generate SPDK bindings");
        let out_path = PathBuf::from(env::var("OUT_DIR").unwrap());
        let bindings_str = bindings.to_string();
        // Strip inner attributes (#![...]) which are invalid inside include!().
        let filtered: String = bindings_str
            .lines()
            .filter(|line| !line.trim_start().starts_with("#!["))
            .collect::<Vec<_>>()
            .join("\n");
        std::fs::write(out_path.join("spdk_bindings.rs"), filtered)
            .expect("Couldn't write bindings");
    }
}

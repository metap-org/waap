fn main() -> Result<(), Box<dyn std::error::Error>> {
    println!("cargo:rerun-if-changed=../proto/inspect.proto");
    tonic_prost_build::configure().build_server(false).compile_protos(&["../proto/inspect.proto"], &["../proto"])?;
    Ok(())
}

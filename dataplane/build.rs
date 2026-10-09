fn main() -> Result<(), Box<dyn std::error::Error>> {
    tonic_build::configure().compile_protos(&["../proto/service.proto"], &["../proto"])?;
    Ok(())
}

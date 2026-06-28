package main

// Version is injected at build time via -ldflags "-X tdrive.Version=...".
// It must be a writable var (not a const) so the linker can override it.
var Version = "dev"

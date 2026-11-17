# Build toolchain for indoc's tree-sitter WebAssembly modules.
# Pinned by digest so every build uses the same compiler and libc.
FROM ghcr.io/webassembly/wasi-sdk@sha256:fdebde86990902087ecc470bf993ffec4d646e8fbd2e68c290a44120437622d0
RUN apt-get update \
 && apt-get install -y --no-install-recommends git ca-certificates \
 && rm -rf /var/lib/apt/lists/*
WORKDIR /work

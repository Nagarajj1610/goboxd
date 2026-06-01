# =============================================================================
# Stage 1: builder
# Builds nsjail from source (tag 3.4) and compiles the goboxd binary.
# We use Ubuntu 22.04 to ensure shared library compatibility with the runtime.
# =============================================================================
FROM ubuntu:22.04 AS builder

ENV DEBIAN_FRONTEND=noninteractive

# Install all build dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    build-essential \
    pkg-config \
    bison \
    flex \
    protobuf-compiler \
    libprotobuf-dev \
    libnl-3-dev \
    libnl-route-3-dev \
    libcap-dev \
    libseccomp-dev \
    ca-certificates \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Install Go 1.23.0 to match the upstream requirements
ARG GO_VERSION=1.23.0
RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" \
    | tar -C /usr/local -xzf -
ENV PATH="/usr/local/go/bin:${PATH}"

# Build nsjail from source (tag 3.4)
WORKDIR /build/nsjail
RUN git clone --branch 3.4 --depth 1 https://github.com/google/nsjail . \
    && make -j$(nproc) \
    && cp nsjail /usr/local/bin/nsjail

# Install golangci-lint for the build/tools stage
RUN curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b /usr/local/bin v1.60.3

# Build goboxd binary
WORKDIR /build/goboxd
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-X main.Version=0.1.0 -X main.GitCommit=$(git rev-parse --short HEAD 2>/dev/null || echo unknown)" \
    -o goboxd \
    ./cmd/goboxd

# =============================================================================
# Stage 2: runtime
# The final image containing all languages, nsjail, and goboxd.
# =============================================================================
FROM ubuntu:22.04 AS runtime

ENV DEBIAN_FRONTEND=noninteractive

# Install all required language compilers and runtimes.
RUN apt-get update && apt-get install -y --no-install-recommends \
    # Python 3
    python3 \
    # C/C++
    g++ \
    gcc \
    # Java
    default-jdk \
    # JavaScript
    nodejs \
    npm \
    # Bash
    bash \
    # Verilog
    iverilog \
    # Ruby
    ruby \
    # Shared libraries for nsjail
    libnl-3-200 \
    libnl-route-3-200 \
    libprotobuf23 \
    libcap2 \
    libseccomp2 \
    # curl and certs for general utilities
    curl \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Install Rust
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs \
    | sh -s -- -y --default-toolchain stable --no-modify-path
ENV PATH="/root/.cargo/bin:${PATH}"
# Create a symlink so rustc is accessible at the standard path
RUN ln -s /root/.cargo/bin/rustc /usr/local/bin/rustc

# Install Kotlin
ARG KOTLIN_VERSION=1.9.23
RUN curl -fsSL "https://github.com/JetBrains/kotlin/releases/download/v${KOTLIN_VERSION}/kotlin-compiler-${KOTLIN_VERSION}.zip" \
    -o /tmp/kotlin.zip \
    && apt-get update && apt-get install -y --no-install-recommends unzip \
    && unzip /tmp/kotlin.zip -d /usr/local \
    && ln -s /usr/local/kotlinc/bin/kotlinc /usr/local/bin/kotlinc \
    && rm /tmp/kotlin.zip \
    && rm -rf /var/lib/apt/lists/*

# Copy nsjail binary from builder stage
COPY --from=builder /build/nsjail/nsjail /usr/sbin/nsjail
RUN chmod 755 /usr/sbin/nsjail

# Copy compiled goboxd binary
COPY --from=builder /build/goboxd/goboxd /app/goboxd
RUN chmod 755 /app/goboxd

# Copy configurations
COPY configs/ /app/configs/

# Setup the jails directory
RUN mkdir -p /var/jails && chmod 1777 /var/jails

WORKDIR /app

EXPOSE 8080

ENV JAIL_DIR=/var/jails
ENV MAX_CONCURRENT_JOBS=4
ENV LOG_LEVEL=info
ENV CONFIG_PATH=/app/configs/languages.yaml

ENTRYPOINT ["./goboxd"]

# Dockerfile for hideas server-side deployment.
#
# This image does NOT build from source. It downloads the matching prebuilt
# release tarball from GitHub Releases (the same artifact produced by
# .github/workflows/release.yml and consumed by scripts/install.sh). Because the
# release binary is built with cgo against glibc on Ubuntu, the runtime base
# must also be a glibc distribution; debian-slim is small and compatible.
#
# Build args:
#   HIDEAS_VERSION   release tag to fetch, e.g. v0.2 (default: latest)
#   HIDEAS_REPO      GitHub repo, default lhanlhanlhan/hideas
#   TARGETARCH       provided automatically by `docker buildx` (amd64 / arm64)

FROM debian:bookworm-slim

ARG HIDEAS_VERSION=latest
ARG HIDEAS_REPO=lhanlhanlhan/hideas
ARG TARGETARCH

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        tar \
    && rm -rf /var/lib/apt/lists/*

RUN set -eu; \
    case "${TARGETARCH:-amd64}" in \
        amd64) asset="hideas-linux-amd64.tar.gz" ;; \
        arm64) asset="hideas-linux-arm64.tar.gz" ;; \
        *) echo "unsupported TARGETARCH: ${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    if [ "${HIDEAS_VERSION}" = "latest" ]; then \
        base_url="https://github.com/${HIDEAS_REPO}/releases/latest/download"; \
    else \
        base_url="https://github.com/${HIDEAS_REPO}/releases/download/${HIDEAS_VERSION}"; \
    fi; \
    curl -fsSL "${base_url}/${asset}" -o /tmp/hideas.tar.gz; \
    if curl -fsSL "${base_url}/SHA256SUMS" -o /tmp/SHA256SUMS 2>/dev/null; then \
        expected=$(grep " ${asset}\$" /tmp/SHA256SUMS | awk '{print $1}'); \
        if [ -n "${expected}" ]; then \
            actual=$(sha256sum /tmp/hideas.tar.gz | awk '{print $1}'); \
            [ "${expected}" = "${actual}" ] || { echo "checksum mismatch" >&2; exit 1; }; \
        fi; \
    fi; \
    tar -xzf /tmp/hideas.tar.gz -C /usr/local/bin; \
    chmod +x /usr/local/bin/hideas; \
    rm -rf /tmp/hideas.tar.gz /tmp/SHA256SUMS

RUN groupadd --system hideas \
    && useradd --system --gid hideas --no-create-home --shell /usr/sbin/nologin hideas \
    && mkdir -p /data /etc/hideas \
    && chown -R hideas:hideas /data /etc/hideas

USER hideas
VOLUME ["/data", "/etc/hideas"]
EXPOSE 8765
ENV HIDEAS_CONFIG=/etc/hideas/config
ENTRYPOINT ["/usr/local/bin/hideas"]
CMD ["serve"]

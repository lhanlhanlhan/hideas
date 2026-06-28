#!/usr/bin/env sh
# scripts/deploy.sh
#
# Interactive deployment helper for hideas. Asks the operator for the
# deployment directory, SSO, and binding details, renders a TOML config file
# and a docker-compose.yml, builds the Docker image, and optionally starts the
# stack with `docker compose up -d`.
#
# Usage (run locally or via curl | sh):
#   curl -fsSL https://raw.githubusercontent.com/lhanlhanlhan/hideas/main/scripts/deploy.sh | sh
#   ./scripts/deploy.sh
#   ./scripts/deploy.sh /opt/hideas        # optional explicit deployment dir
#
# All generated files are placed under the deployment directory:
#   <dir>/config            hideas config consumed by `hideas serve`
#   <dir>/docker-compose.yml
#   <dir>/data/             persistent SQLite database directory
#
# The script is POSIX sh; it relies on docker (with the compose plugin) being
# installed on the host. When run via `curl | sh`, it does NOT require a local
# clone of the hideas repository — the Dockerfile is fetched from GitHub on
# demand.

set -eu

REPO_DEFAULT="lhanlhanlhan/hideas"
REPO=${HIDEAS_REPO:-$REPO_DEFAULT}
REF=${HIDEAS_REF:-main}
IMAGE_NAME=${HIDEAS_IMAGE:-hideas:latest}

usage() {
    cat >&2 <<'EOF'
Usage:
  curl -fsSL https://raw.githubusercontent.com/lhanlhanlhan/hideas/main/scripts/deploy.sh | sh
  ./scripts/deploy.sh [deployment-dir]

Environment:
  HIDEAS_REPO   GitHub repo to fetch release & Dockerfile from
                (default: lhanlhanlhan/hideas)
  HIDEAS_REF    Git ref to fetch Dockerfile from (default: main)
  HIDEAS_IMAGE  Docker image tag to build (default: hideas:latest)
EOF
}

if [ "$#" -ge 1 ]; then
    case "$1" in
        -h|--help) usage; exit 0 ;;
    esac
fi

DEPLOY_DIR_ARG=${1:-}

prompt() {
    label=$1
    default=$2
    if [ -n "$default" ]; then
        printf "%s [%s]: " "$label" "$default" > /dev/tty
    else
        printf "%s: " "$label" > /dev/tty
    fi
    IFS= read -r answer < /dev/tty || answer=""
    if [ -z "$answer" ]; then
        answer=$default
    fi
    printf "%s" "$answer"
}

prompt_required() {
    label=$1
    while :; do
        value=$(prompt "$label" "")
        if [ -n "$value" ]; then
            printf "%s" "$value"
            return
        fi
        printf "  (value is required)\n" > /dev/tty
    done
}

prompt_secret() {
    label=$1
    stty -echo < /dev/tty 2>/dev/null || true
    printf "%s: " "$label" > /dev/tty
    IFS= read -r value < /dev/tty || value=""
    stty echo < /dev/tty 2>/dev/null || true
    printf "\n" > /dev/tty
    printf "%s" "$value"
}

prompt_yes_no() {
    label=$1
    default=$2
    while :; do
        answer=$(prompt "$label (y/n)" "$default")
        case "$answer" in
            y|Y|yes|YES) printf "yes"; return ;;
            n|N|no|NO)   printf "no";  return ;;
        esac
    done
}

random_token() {
    # 32-byte base64-url-safe random string; fall back to /dev/urandom hex.
    if command -v openssl >/dev/null 2>&1; then
        openssl rand -base64 32 | tr -d '\n' | tr '+/' '-_' | tr -d '='
    else
        head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n'
    fi
}

# Strip a trailing slash, if any. POSIX-safe.
trim_trailing_slash() {
    case "$1" in
        */) printf "%s" "${1%/}" ;;
        *)  printf "%s" "$1" ;;
    esac
}

echo
echo "hideas deployment configuration"
echo "==============================="
echo

REUSE_EXISTING=no
# If the script is invoked from a directory that already contains a previous
# deployment (both files present), offer to reuse it instead of asking every
# question again. This is the common path when the operator just wants to
# rebuild the image or bring the stack up.
if [ -z "$DEPLOY_DIR_ARG" ] && [ -f "$(pwd)/config" ] && [ -f "$(pwd)/docker-compose.yml" ]; then
    echo "Found an existing deployment in: $(pwd)"
    echo "  - config"
    echo "  - docker-compose.yml"
    REUSE_ANS=$(prompt_yes_no "Reuse this directory and skip configuration?" "y")
    if [ "$REUSE_ANS" = "yes" ]; then
        REUSE_EXISTING=yes
        DEPLOY_DIR=$(pwd)
    fi
fi

if [ "$REUSE_EXISTING" = "no" ]; then
    if [ -n "$DEPLOY_DIR_ARG" ]; then
        DEPLOY_DIR=$DEPLOY_DIR_ARG
    else
        DEPLOY_DIR=$(prompt_required "Deployment directory (config + docker-compose.yml + data/ live here)")
    fi
    case "$DEPLOY_DIR" in
        /*) : ;;
        ~/*|"~") DEPLOY_DIR="${HOME}${DEPLOY_DIR#~}" ;;
        *)  DEPLOY_DIR="$(pwd)/$DEPLOY_DIR" ;;
    esac
fi
echo "Deployment directory: ${DEPLOY_DIR}"
echo

CONFIG_PATH="$DEPLOY_DIR/config"
COMPOSE_PATH="$DEPLOY_DIR/docker-compose.yml"

if [ "$REUSE_EXISTING" = "yes" ]; then
    echo "Reusing existing ${CONFIG_PATH} and ${COMPOSE_PATH}."
    echo
else

PUBLIC_BASE_URL=$(prompt_required "Public base URL the SSO will redirect to (e.g. https://hideas.example.com/hideas/)")
PUBLIC_BASE_URL=$(trim_trailing_slash "$PUBLIC_BASE_URL")
BASE_PATH=$(prompt "HTTP base_path (must match the URL above)" "/")
case "$BASE_PATH" in
    /*) : ;;
    *)  BASE_PATH="/$BASE_PATH" ;;
esac
case "$BASE_PATH" in
    */) : ;;
    *)  BASE_PATH="$BASE_PATH/" ;;
esac

HOST=$(prompt "Container listen host (use 0.0.0.0 inside Docker)" "0.0.0.0")
PORT=$(prompt "Container listen port" "8765")
PUBLISH_PORT=$(prompt "Host port to publish on" "$PORT")

SSO_ISSUER=$(prompt_required "SSO issuer URL (e.g. https://sso.example.com/oauth)")
SSO_ISSUER=$(trim_trailing_slash "$SSO_ISSUER")
SSO_CLIENT_ID=$(prompt_required "SSO client_id")
SSO_CLIENT_SECRET=$(prompt_secret "SSO client_secret")
SSO_SCOPES=$(prompt "SSO scopes" "openid profile email")

WANT_STATIC=$(prompt_yes_no "Generate a static bearer token (for CI / emergency access)?" "n")
if [ "$WANT_STATIC" = "yes" ]; then
    STATIC_TOKEN=$(random_token)
else
    STATIC_TOKEN=""
fi

# Compute the SSO redirect URL from the public base URL plus the API callback
# path. The hideas server validates this on startup.
REDIRECT_URL="${PUBLIC_BASE_URL}${BASE_PATH#/}api/v1/auth/callback"

mkdir -p "$DEPLOY_DIR" "$DEPLOY_DIR/data"

# Escape backslashes and double quotes for safe TOML emission.
toml_escape() {
    printf "%s" "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g'
}

{
    echo "# Generated by scripts/deploy.sh on $(date -u +%Y-%m-%dT%H:%M:%SZ)."
    echo "db = \"/data/hideas.sqlite\""
    echo "host = \"$(toml_escape "$HOST")\""
    echo "port = $PORT"
    echo "base_path = \"$(toml_escape "$BASE_PATH")\""
    if [ -n "$STATIC_TOKEN" ]; then
        echo "token = \"$(toml_escape "$STATIC_TOKEN")\""
    fi
    echo
    echo "[sso]"
    echo "issuer = \"$(toml_escape "$SSO_ISSUER")\""
    echo "client_id = \"$(toml_escape "$SSO_CLIENT_ID")\""
    echo "client_secret = \"$(toml_escape "$SSO_CLIENT_SECRET")\""
    echo "redirect_url = \"$(toml_escape "$REDIRECT_URL")\""
    echo "scopes = \"$(toml_escape "$SSO_SCOPES")\""
} > "$CONFIG_PATH"
chmod 600 "$CONFIG_PATH"

{
    echo "services:"
    echo "  hideas:"
    echo "    image: $IMAGE_NAME"
    echo "    container_name: hideas"
    echo "    restart: unless-stopped"
    echo "    ports:"
    echo "      - \"${PUBLISH_PORT}:${PORT}\""
    echo "    volumes:"
    echo "      - ./config:/etc/hideas/config:ro"
    echo "      - ./data:/data"
} > "$COMPOSE_PATH"

echo
echo "Wrote ${CONFIG_PATH}"
echo "Wrote ${COMPOSE_PATH}"
echo "Redirect URL to register with your SSO:"
echo "  ${REDIRECT_URL}"
if [ -n "$STATIC_TOKEN" ]; then
    echo
    echo "Generated static bearer token (store it somewhere safe; it is also"
    echo "written to ${CONFIG_PATH}):"
    echo "  ${STATIC_TOKEN}"
fi

fi  # end of REUSE_EXISTING=no branch

BUILD_NOW=$(prompt_yes_no "Build the Docker image now (${IMAGE_NAME})?" "y")
if [ "$BUILD_NOW" = "yes" ]; then
    HIDEAS_VERSION=$(prompt "Release version to fetch (or 'latest')" "latest")
    HIDEAS_REPO=$(prompt "GitHub repo to fetch the release from" "$REPO")
    # Build a tiny context containing only the Dockerfile, fetched from GitHub
    # when the script is run via `curl | sh` outside a clone. The Dockerfile
    # itself pulls the prebuilt release tarball at image build time.
    build_ctx=$(mktemp -d)
    trap 'rm -rf "$build_ctx"' EXIT INT TERM
    if ! curl -fsSL "https://raw.githubusercontent.com/${REPO}/${REF}/Dockerfile" -o "$build_ctx/Dockerfile"; then
        echo "failed to fetch Dockerfile from ${REPO}@${REF}" >&2
        exit 1
    fi
    docker build \
        --build-arg HIDEAS_VERSION="$HIDEAS_VERSION" \
        --build-arg HIDEAS_REPO="$HIDEAS_REPO" \
        -t "$IMAGE_NAME" \
        "$build_ctx"
fi

START_NOW=$(prompt_yes_no "Start the stack with docker compose now?" "n")
if [ "$START_NOW" = "yes" ]; then
    # Prefer the Compose v2 plugin (`docker compose`). Fall back to the legacy
    # standalone binary (`docker-compose`) when the plugin is not installed.
    if docker compose version >/dev/null 2>&1; then
        compose_cmd="docker compose"
    elif command -v docker-compose >/dev/null 2>&1; then
        compose_cmd="docker-compose"
    else
        echo "Neither 'docker compose' (v2 plugin) nor 'docker-compose' (v1) is" >&2
        echo "available. Install one and rerun:" >&2
        echo "  cd ${DEPLOY_DIR} && docker compose up -d" >&2
        exit 1
    fi
    (cd "$DEPLOY_DIR" && $compose_cmd up -d)
    echo
    echo "Stack started. Useful next commands:"
    echo "  ${compose_cmd} -f ${COMPOSE_PATH} logs -f"
    echo "  ${compose_cmd} -f ${COMPOSE_PATH} down"
else
    echo
    echo "To start later:"
    echo "  cd ${DEPLOY_DIR} && docker compose up -d   # or: docker-compose up -d"
fi

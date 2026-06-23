#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_NAME="mcp-searxng-go"
BINARY_NAME="mcp-server"
LOCAL_BINARY="${ROOT_DIR}/build/${BINARY_NAME}"
LOCAL_CONFIG="${ROOT_DIR}/configs/config.yaml"
LOCAL_ENV="${ROOT_DIR}/.env"

REMOTE_HOST="${REMOTE_HOST:-yfms@192.168.4.70}"
REMOTE_BASE_DIR="${REMOTE_BASE_DIR:-/home/yfms/docker/docker-searxng}"
REMOTE_BINARY_DIR="${REMOTE_BASE_DIR}/bin"
REMOTE_CONFIG_DIR="${REMOTE_BASE_DIR}/configs"
REMOTE_SERVICE_NAME="${REMOTE_SERVICE_NAME:-${APP_NAME}.service}"
REMOTE_SERVICE_DIR='~/.config/systemd/user'
REMOTE_BINARY_PATH="${REMOTE_BINARY_DIR}/${BINARY_NAME}"
REMOTE_CONFIG_PATH="${REMOTE_CONFIG_DIR}/config.yaml"
REMOTE_LOG_DIR="${REMOTE_BASE_DIR}/logs"
REMOTE_ENV_FILE="${REMOTE_BASE_DIR}/${APP_NAME}.env"
SERVICE_HEALTH_URL="${SERVICE_HEALTH_URL:-http://127.0.0.1:7778/healthz}"
REMOTE_BINARY_TMP_PATH="${REMOTE_BINARY_PATH}.tmp"
REMOTE_CONFIG_TMP_PATH="${REMOTE_CONFIG_PATH}.tmp"
REMOTE_ENV_TMP_PATH="${REMOTE_ENV_FILE}.tmp"

usage() {
  cat <<EOF
Usage: ./deploy.sh <command>

Commands:
  deploy    Build locally, upload binary/config, install service, restart it
  install   Upload assets and install/update the service without rebuilding
  start     Start the remote user service
  stop      Stop the remote user service
  restart   Restart the remote user service
  status    Show remote service status
  logs      Tail recent remote service logs
  health    Hit the remote /healthz endpoint
  service   Print the rendered systemd user unit

Environment overrides:
  REMOTE_HOST          Default: ${REMOTE_HOST}
  REMOTE_BASE_DIR      Default: ${REMOTE_BASE_DIR}
  REMOTE_SERVICE_NAME  Default: ${REMOTE_SERVICE_NAME}

Notes:
  - This script installs a systemd user service for ${REMOTE_HOST}.
  - For the service to survive logout/reboot, the remote host should have lingering enabled:
      sudo loginctl enable-linger yfms
EOF
}

service_unit() {
  cat <<EOF
[Unit]
Description=${APP_NAME} MCP server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=${REMOTE_BASE_DIR}
EnvironmentFile=-${REMOTE_ENV_FILE}
ExecStart=${REMOTE_BINARY_PATH} --config ${REMOTE_CONFIG_PATH}
Restart=on-failure
RestartSec=5
StandardOutput=append:${REMOTE_LOG_DIR}/stdout.log
StandardError=append:${REMOTE_LOG_DIR}/stderr.log

[Install]
WantedBy=default.target
EOF
}

run_remote() {
  ssh "${REMOTE_HOST}" "$@"
}

build_local() {
  mkdir -p "${ROOT_DIR}/.cache/go-build" "${ROOT_DIR}/build"
  GOCACHE="${ROOT_DIR}/.cache/go-build" go build -trimpath -buildvcs=false -o "${LOCAL_BINARY}" ./cmd/server
}

ensure_local_files() {
  if [[ ! -f "${LOCAL_BINARY}" ]]; then
    echo "missing binary: ${LOCAL_BINARY}" >&2
    exit 1
  fi
  if [[ ! -f "${LOCAL_CONFIG}" ]]; then
    echo "missing config: ${LOCAL_CONFIG}" >&2
    exit 1
  fi
}

prepare_remote() {
  run_remote "mkdir -p '${REMOTE_BINARY_DIR}' '${REMOTE_CONFIG_DIR}' '${REMOTE_LOG_DIR}' ${REMOTE_SERVICE_DIR}"
}

upload_assets() {
  ensure_local_files
  scp "${LOCAL_BINARY}" "${REMOTE_HOST}:${REMOTE_BINARY_TMP_PATH}"
  scp "${LOCAL_CONFIG}" "${REMOTE_HOST}:${REMOTE_CONFIG_TMP_PATH}"
  run_remote "mv '${REMOTE_BINARY_TMP_PATH}' '${REMOTE_BINARY_PATH}' && chmod 775 '${REMOTE_BINARY_PATH}'"
  run_remote "mv '${REMOTE_CONFIG_TMP_PATH}' '${REMOTE_CONFIG_PATH}'"
  upload_env
  service_unit | ssh "${REMOTE_HOST}" "cat > ${REMOTE_BASE_DIR}/${REMOTE_SERVICE_NAME}"
}

upload_env() {
  if [[ ! -f "${LOCAL_ENV}" ]]; then
    echo "no local .env at ${LOCAL_ENV}; skipping secret upload (BRAVE_SEARCH_API will be unset)" >&2
    return 0
  fi
  scp "${LOCAL_ENV}" "${REMOTE_HOST}:${REMOTE_ENV_TMP_PATH}"
  run_remote "mv '${REMOTE_ENV_TMP_PATH}' '${REMOTE_ENV_FILE}' && chmod 600 '${REMOTE_ENV_FILE}'"
}

install_service() {
  run_remote "mkdir -p ${REMOTE_SERVICE_DIR}"
  run_remote "cp '${REMOTE_BASE_DIR}/${REMOTE_SERVICE_NAME}' ${REMOTE_SERVICE_DIR}/${REMOTE_SERVICE_NAME}"
  run_remote "systemctl --user daemon-reload"
  run_remote "systemctl --user enable '${REMOTE_SERVICE_NAME}'"
}

restart_service() {
  run_remote "systemctl --user restart '${REMOTE_SERVICE_NAME}'"
}

start_service() {
  run_remote "systemctl --user start '${REMOTE_SERVICE_NAME}'"
}

stop_service() {
  run_remote "systemctl --user stop '${REMOTE_SERVICE_NAME}' || true"
}

status_service() {
  run_remote "systemctl --user --no-pager --full status '${REMOTE_SERVICE_NAME}'"
}

logs_service() {
  run_remote "journalctl --user -u '${REMOTE_SERVICE_NAME}' -n 200 --no-pager"
}

health_check() {
  run_remote "curl -fsS '${SERVICE_HEALTH_URL}'"
}

deploy() {
  build_local
  prepare_remote
  stop_service
  upload_assets
  install_service
  start_service
  status_service
}

install_only() {
  prepare_remote
  upload_assets
  install_service
  status_service
}

main() {
  local command="${1:-}"
  case "${command}" in
    deploy)
      deploy
      ;;
    install)
      install_only
      ;;
    start)
      start_service
      ;;
    stop)
      stop_service
      ;;
    restart)
      restart_service
      ;;
    status)
      status_service
      ;;
    logs)
      logs_service
      ;;
    health)
      health_check
      ;;
    service)
      service_unit
      ;;
    ""|-h|--help|help)
      usage
      ;;
    *)
      echo "unknown command: ${command}" >&2
      usage
      exit 1
      ;;
  esac
}

main "$@"

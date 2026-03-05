#!/usr/bin/env bash
set -euo pipefail

timestamp() {
  date +"%Y-%m-%d %T"
}

info() {
  local flag
  flag="$(timestamp)"
  echo -e "\033[36m INFO [$flag] >> $* \033[0m"
}

warn() {
  local flag
  flag="$(timestamp)"
  echo -e "\033[33m WARN [$flag] >> $* \033[0m"
}

error() {
  local flag
  flag="$(timestamp)"
  echo -e "\033[31m ERROR [$flag] >> $* \033[0m"
  exit 1
}

RELEASE_NAME="${RELEASE_NAME:-kite}"
NAMESPACE="${NAMESPACE:-kite-system}"
HELM_OPTS="${HELM_OPTS:-}"
ENABLE_APP="${ENABLE_APP:-true}"

get_sealos_config() {
  local key=$1
  kubectl get configmap sealos-config -n sealos-system -o "jsonpath={.data.${key}}"
}

sealos_jwt_secret="$(get_sealos_config jwtInternal || true)"
sealos_cloud_domain="$(get_sealos_config cloudDomain || true)"

[ -n "${sealos_jwt_secret}" ] || error "Failed to read sealos-config.data.jwtInternal"
[ -n "${sealos_cloud_domain}" ] || error "Failed to read sealos-config.data.cloudDomain"

jwt_secret="$(openssl rand -hex 32)"
encrypt_key="$(openssl rand -hex 32)"

helm_set_args=(
  --set-string "jwtSecret=${jwt_secret}"
  --set-string "encryptKey=${encrypt_key}"
  --set-string "sealos.jwtSecret=${sealos_jwt_secret}"
  --set-string "cloudDomain=${sealos_cloud_domain}"
)

if [ "${ENABLE_APP}" = "true" ]; then
  helm_set_args+=(--set "app.enabled=true")
fi

node_count="$(kubectl get nodes --no-headers 2>/dev/null | wc -l | tr -d ' ')"
if [ "${node_count}" = "1" ]; then
  warn "Single-node cluster detected, force app/database replicas to 1."
  helm_set_args+=(
    --set "replicaCount=1"
    --set "db.postgres.native.replicas=1"
  )
fi

helm_opts_arr=()
if [ -n "${HELM_OPTS}" ]; then
  # shellcheck disable=SC2206
  helm_opts_arr=(${HELM_OPTS})
fi

info "Installing chart charts/kite into namespace ${NAMESPACE}"
helm upgrade -i "${RELEASE_NAME}" -n "${NAMESPACE}" --create-namespace charts/kite \
  "${helm_set_args[@]}" \
  "${helm_opts_arr[@]}" \
  --wait

#!/usr/bin/env bash
set -euo pipefail

kubectl -n default create secret generic mount-options \
  --from-literal=mountOptions='nfsvers=4.1' \
  --from-literal=krb-pwd='password!' \
  --dry-run=client -o yaml |
kubectl apply -f -

kubectl apply -f deploy/example/nfs-provisioner/nfs-server.yaml
kubectl apply -f deploy/example/nfs-provisioner/nfs-krb-server.yaml

kubectl -n default rollout status deployment/nfs-krb-server \
  --timeout=120s

server_pod="$(
  kubectl -n default get pod \
    -l app=nfs-krb-server \
    -o jsonpath='{.items[0].metadata.name}'
)"

test_dir="$(mktemp -d)"
trap 'rm -rf "$test_dir"' EXIT

kubectl -n default exec "$server_pod" -- \
  cat /shared/krb5.conf >"$test_dir/krb5.conf"

kubectl -n default exec "$server_pod" -- \
  cat /shared/client.keytab >"$test_dir/client.keytab"

# CSI secrets are map<string,string>. Encode the binary keytab explicitly
# so it remains valid text while passing through CSI.
base64 -w0 "$test_dir/client.keytab" >"$test_dir/client.keytab.b64"

kubectl -n default create secret generic mount-options \
  --from-literal=mountOptions='nfsvers=4.1' \
  --from-literal=krb-pwd='password!' \
  --from-file=krb5.conf="$test_dir/krb5.conf" \
  --from-file=client.keytab.b64="$test_dir/client.keytab.b64" \
  --dry-run=client -o yaml |
kubectl apply -f -

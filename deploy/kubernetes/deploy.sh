#!/usr/bin/env bash

# This script captures the steps required to successfully
# deploy the hostpath plugin driver.  This should be considered
# authoritative and all updates for this process should be
# done here and referenced elsewhere.

# The script assumes that kubectl is available on the OS path 
# where it is executed.

set -e
set -o pipefail

BASE_DIR=$(dirname "$0")

# If set, the following env variables override image registry and/or tag for each of the images.
# They are named after the image name, with hyphen replaced by underscore and in upper case.
#
# - CSI_ATTACHER_REGISTRY
# - CSI_ATTACHER_TAG
# - CSI_NODE_DRIVER_REGISTRAR_REGISTRY
# - CSI_NODE_DRIVER_REGISTRAR_TAG
# - NFSPLUGIN_REGISTRY
# - NFSPLUGIN_TAG
#
# Alternatively, it is possible to override all registries or tags with:
# - IMAGE_REGISTRY
# - IMAGE_TAG
# These are used as fallback when the more specific variables are unset or empty.
#
# Beware that the .yaml files do not have "imagePullPolicy: Always". That means that
# also the "canary" images will only be pulled once. This is good for testing
# (starting a pod multiple times will always run with the same canary image), but
# implies that refreshing that image has to be done manually.
#
# As a special case, 'none' as registry removes the registry name.

# The default is to use the RBAC rules that match the image that is
# being used, also in the case that the image gets overridden. This
# way if there are breaking changes in the RBAC rules, the deployment
# will continue to work.
#
# However, such breaking changes should be rare and only occur when updating
# to a new major version of a sidecar. Nonetheless, to allow testing the scenario
# where the image gets overridden but not the RBAC rules, updating the RBAC
# rules can be disabled.
: ${UPDATE_RBAC_RULES:=true}
function rbac_version () {
    yaml="$1"
    image="$2"
    update_rbac="$3"

    # get version from `image: quay.io/k8scsi/csi-attacher:v1.0.1`, ignoring comments
    version="$(sed -e 's/ *#.*$//' "$yaml" | grep "image:.*$image" | sed -e 's/ *#.*//' -e 's/.*://')"

    if $update_rbac; then
        # apply overrides
        varname=$(echo $image | tr - _ | tr a-z A-Z)
        eval version=\${${varname}_TAG:-\${IMAGE_TAG:-\$version}}
    fi

    # When using canary images, we have to assume that the
    # canary images were built from the corresponding branch.
    case "$version" in canary) version=master;;
                        *-canary) version="$(echo "$version" | sed -e 's/\(.*\)-canary/release-\1/')";;
    esac

    echo "$version"
}

# In addition, the RBAC rules can be overridden separately.
CSI_ATTACHER_RBAC_YAML="https://raw.githubusercontent.com/kubernetes-csi/external-attacher/$(rbac_version "${BASE_DIR}/csi-attacher-nfsplugin.yaml" csi-attacher false)/deploy/kubernetes/rbac.yaml"
: ${CSI_ATTACHER_RBAC:=https://raw.githubusercontent.com/kubernetes-csi/external-attacher/$(rbac_version "${BASE_DIR}/csi-attacher-nfsplugin.yaml" csi-attacher "${UPDATE_RBAC_RULES}")/deploy/kubernetes/rbac.yaml}

INSTALL_CRD=${INSTALL_CRD:-"false"}

# Some images are not affected by *_REGISTRY/*_TAG and IMAGE_* variables.
# The default is to update unless explicitly excluded.
update_image () {
    case "$1" in socat) return 1;; esac
}

run () {
    echo "$@" >&2
    "$@"
}

# rbac rules
echo "applying RBAC rules"
for component in CSI_ATTACHER; do
    eval current="\${${component}_RBAC}"
    eval original="\${${component}_RBAC_YAML}"
    if [ "$current" != "$original" ]; then
        echo "Using non-default RBAC rules for $component. Changes from $original to $current are:"
        diff -c <(wget --quiet -O - "$original") <(if [[ "$current" =~ ^http ]]; then wget --quiet -O - "$current"; else cat "$current"; fi) || true
    fi
    run kubectl apply -f "${current}"
done

# deploy nfs plugin and registrar sidecar
echo "deploying nfs plugin components"
for i in $(ls ${BASE_DIR}/*.yaml | sort); do
    echo "   $i"
    modified="$(cat "$i" | while IFS= read -r line; do
        nocomments="$(echo "$line" | sed -e 's/ *#.*$//')"
        if echo "$nocomments" | grep -q '^[[:space:]]*image:[[:space:]]*'; then
            # Split 'image: quay.io/k8scsi/csi-attacher:v1.0.1'
            # into image (quay.io/k8scsi/csi-attacher:v1.0.1),
            # registry (quay.io/k8scsi),
            # name (csi-attacher),
            # tag (v1.0.1).
            image=$(echo "$nocomments" | sed -e 's;.*image:[[:space:]]*;;')
            registry=$(echo "$image" | sed -e 's;\(.*\)/.*;\1;')
            name=$(echo "$image" | sed -e 's;.*/\([^:]*\).*;\1;')
            tag=$(echo "$image" | sed -e 's;.*:;;')

            # Variables are with underscores and upper case.
            varname=$(echo $name | tr - _ | tr a-z A-Z)

            # Now replace registry and/or tag, if set as env variables.
            # If not set, the replacement is the same as the original value.
            # Only do this for the images which are meant to be configurable.
            if update_image "$name"; then
                prefix=$(eval echo \${${varname}_REGISTRY:-${IMAGE_REGISTRY:-${registry}}}/ | sed -e 's;none/;;')
                suffix=$(eval echo :\${${varname}_TAG:-${IMAGE_TAG:-${tag}}})
                line="$(echo "$nocomments" | sed -e "s;$image;${prefix}${name}${suffix};")"
            fi
            echo "        using $line" >&2
        fi
        echo "$line"
    done)"
    if ! echo "$modified" | kubectl apply -f -; then
        echo "modified version of $i:"
        echo "$modified"
        exit 1
    fi
done

# Wait until all pods are running. We have to make some assumptions
# about the deployment here, otherwise we wouldn't know what to wait
# for: the expectation is that we run attacher and nfs plugin in the default namespace.
cnt=0
while [ $(kubectl get pods 2>/dev/null | grep '^csi-.*nfsplugin.* Running ' | wc -l) -lt 2 ];  do
    if [ $cnt -gt 30 ]; then
        echo "Running pods:"
        kubectl describe pods

        echo >&2 "ERROR: nfs deployment not ready after over 5min"
        exit 1
    fi
    echo $(date +%H:%M:%S) "waiting for nfs deployment to complete, attempt #$cnt"
    cnt=$(($cnt + 1))
    sleep 10
done

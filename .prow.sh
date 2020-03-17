#! /bin/bash

# A Prow job can override these defaults, but this shouldn't be necessary.

# Only these tests make sense for csi-driver-nfs until we can integrate k/k
# e2es.
: ${CSI_PROW_TESTS:="unit"}

. release-tools/prow.sh

main

#! /bin/bash

# Only these tests work for csi-drivers-flex, E2E and sanity testing
# will require further work.
: ${CSI_PROW_TESTS:="unit serial parallel serial-alpha parallel-alpha"}

# Customize deployment and E2E testing.
: ${CSI_PROW_DEPLOY_SCRIPT:="deploy.sh"}
: ${CSI_PROW_DEPLOYMENT:="kubernetes"}
: ${CSI_PROW_E2E_TEST_PREFIX:="CSI Volumes"}

. release-tools/prow.sh

# Install custom E2E test suite as bin/tests.
install_e2e () {
    if [ -e "${CSI_PROW_WORK}/e2e.test" ]; then
        return
    fi

    make build-tests && cp bin/tests "${CSI_PROW_WORK}/e2e.test"
}

# Invoke the custom E2E test suite for a certain subset of the tests (serial, parallel, ...)
run_e2e () (
    name="$1"
    shift

    install_e2e || die "building e2e.test failed"
    install_ginkgo || die "installing ginkgo failed"

    trap "move_junit '$name'" EXIT

    cd "${GOPATH}/src/${CSI_PROW_E2E_IMPORT_PATH}" &&
    run_with_loggers ginkgo -v "$@" "${CSI_PROW_WORK}/e2e.test" -- -report-dir "${ARTIFACTS}"
)

main

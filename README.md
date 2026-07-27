# csi-driver-nfs helm chart repository

This branch hosts the Helm chart repository for csi-driver-nfs via
GitHub Pages. Published automatically by the
`Publish Helm Chart to GitHub Pages` workflow.

To use:

    helm repo add csi-driver-nfs https://kubernetes-csi.github.io/csi-driver-nfs
    helm repo update csi-driver-nfs
    helm install csi-driver-nfs csi-driver-nfs/csi-driver-nfs \
      --namespace kube-system --version <x.y.z>

#!/usr/bin/env python3

# Copyright 2026 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Check orphan-reaping configuration (requires Helm and PyYAML, as does chart CI)."""

from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

import yaml


ROOT = Path(__file__).resolve().parents[1]
CHART = ROOT / "charts/latest/csi-driver-nfs"


class ProcessNamespaceTest(unittest.TestCase):
    def check_workloads(self, documents, node=True):
        expected = {"csi-nfs-controller": None, "csi-nfs-node": node}
        found = set()
        for doc in documents:
            if not doc or doc.get("kind") not in ("Deployment", "DaemonSet"):
                continue
            name = doc["metadata"]["name"]
            spec = doc["spec"]["template"]["spec"]
            if name not in expected:
                # The standalone snapshot controller does not mount NFS.
                self.assertNotIn("shareProcessNamespace", spec)
                continue
            found.add(name)
            if expected[name] is None:
                self.assertNotIn("shareProcessNamespace", spec, name)
            else:
                self.assertIs(spec.get("shareProcessNamespace"), expected[name], name)
            # Sharing the host PID namespace is incompatible with Pod PID sharing.
            self.assertFalse(spec.get("hostPID", False), name)
        self.assertEqual(found, set(expected))

    def render(self, chart=CHART, *args):
        output = subprocess.check_output(
            ["helm", "template", "test", str(chart), "--namespace", "kube-system", *args],
            text=True,
        )
        return list(yaml.safe_load_all(output))

    def test_defaults(self):
        self.check_workloads(self.render())

    def test_node_overrides(self):
        for node in (True, False):
            with self.subTest(node=node):
                documents = self.render(
                    CHART, "--set",
                    f"node.shareProcessNamespace={str(node).lower()},"
                    "externalSnapshotter.enabled=true",
                )
                self.check_workloads(documents, node=node)

    def test_missing_values(self):
        # Older reused values may not contain the new settings.
        with tempfile.TemporaryDirectory() as tmp:
            chart = Path(tmp) / "chart"
            shutil.copytree(CHART, chart)
            values_path = chart / "values.yaml"
            values = yaml.safe_load(values_path.read_text())
            values["node"].pop("shareProcessNamespace", None)
            values_path.write_text(yaml.safe_dump(values))
            self.check_workloads(self.render(chart))

    def test_static_manifests(self):
        documents = []
        for component in ("controller", "node"):
            path = ROOT / "deploy" / f"csi-nfs-{component}.yaml"
            documents.extend(yaml.safe_load_all(path.read_text()))
        self.check_workloads(documents)


if __name__ == "__main__":
    unittest.main()

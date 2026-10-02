// Copyright 2026 Adobe. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package kafka

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type metadataFixture struct {
	t                           *testing.T
	root, metadata, config, bin string
	data                        []string
	meta                        []string
	env                         []string
}

func newMetadataFixture(t *testing.T, source int) *metadataFixture {
	t.Helper()
	root, err := os.MkdirTemp(".", ".metadata-fixture-")
	require.NoError(t, err)
	root, err = filepath.Abs(root)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(root)) })
	f := &metadataFixture{t: t, root: root, metadata: filepath.Join(root, "metadata"), config: filepath.Join(root, "broker-config"), bin: filepath.Join(root, "bin")}
	f.data = []string{filepath.Join(root, "csi-kafka-logs1"), filepath.Join(root, "csi-kafka-logs2")}
	for _, path := range append(append([]string{}, f.data...), f.metadata, f.bin, filepath.Join(root, "wait")) {
		require.NoError(t, os.MkdirAll(path, 0o755))
	}
	f.write(f.config, fmt.Sprintf("process.roles=broker\nnode.id=1\nmetadata.log.dir=%s/kafka\nlog.dirs=%s/kafka,%s/kafka\n", f.metadata, f.data[0], f.data[1]))
	f.write(filepath.Join(f.bin, "kafka-storage.sh"), `#!/bin/bash
set -eu
while [[ $# -gt 0 ]]; do
  if [[ "$1" == -c ]]; then config=$2; shift; fi
  shift
done
while IFS= read -r line; do
  case "$line" in log.dirs=*) dir=${line#*=};; esac
done < "$config"
mkdir -p "$dir"
if [[ ! -f "$dir/meta.properties" ]]; then
  printf 'version=1\ncluster.id=cluster-one\nnode.id=1\ndirectory.id=%s\n' "${FORMAT_ID:-AAAAAAAAAAAAAAAAAAAAAQ}" > "$dir/meta.properties"
  if [[ "${FAULT_AT:-}" == partial-format ]]; then exit 42; fi
  printf 'generated-bootstrap' > "$dir/bootstrap.checkpoint"
fi
if [[ "${FAULT_AT:-}" == format ]]; then exit 42; fi
`)
	for _, name := range []string{"cp", "mv"} {
		real, err := exec.LookPath(name)
		require.NoError(t, err)
		f.write(filepath.Join(f.bin, name), fmt.Sprintf(`#!/bin/bash
set -eu
if [[ "%s" == cp && "${FAULT_AT:-}" == copy && "$1" == -a && "$2" == */__cluster_metadata-0/. ]]; then
  "%s" -a "$2/00000000000000000000.log" "$3"
  exit 42
fi
"%s" "$@"
if [[ "%s" == mv ]]; then
  for arg in "$@"; do
    if [[ "${FAULT_AT:-}" == manifest && "$arg" == "$METADATA_MOUNT/.koperator-metadata-migration" ]]; then exit 42; fi
    if [[ "${FAULT_AT:-}" == stage-owner && "$arg" == */.koperator-metadata-stage-owner ]]; then exit 42; fi
    if [[ "${FAULT_AT:-}" == publish && "$arg" == "$METADATA_MOUNT/kafka" ]]; then exit 42; fi
    if [[ "${FAULT_AT:-}" == backup && "$arg" == */.koperator-metadata-backup-1 ]]; then exit 42; fi
    if [[ "${FAULT_AT:-}" == complete && "$arg" == "$METADATA_MOUNT/.koperator-metadata-complete" ]]; then exit 42; fi
  done
fi
`, name, real, real, name))
	}
	f.env = append(os.Environ(),
		"KAFKA_HOME="+root, "BROKER_CONFIG="+f.config, "WORK_DIR="+filepath.Join(root, "wait"),
		"METADATA_MOUNT="+f.metadata, "NODE_ID=1", "CLUSTER_ID=cluster-one", "ALLOW_FRESH=true",
		"PATH="+f.bin+":"+os.Getenv("PATH"))
	if source >= 0 {
		for i := range f.data {
			f.setMeta(i, fmt.Sprintf("version=1\ncluster.id=cluster-one\nnode.id=1\ndirectory.id=BBBBBBBBBBBBBBBBBBBBB%d\n", i))
			f.write(filepath.Join(f.data[i], "kafka/orders-0/00000000000000000000.log"), "original-topic-replica")
		}
		f.addSource(source)
	}
	return f
}

func (f *metadataFixture) write(path, content string) {
	f.t.Helper()
	require.NoError(f.t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(f.t, os.WriteFile(path, []byte(content), 0o755))
}

func (f *metadataFixture) setMeta(i int, content string) {
	f.t.Helper()
	for len(f.meta) <= i {
		f.meta = append(f.meta, "")
	}
	f.meta[i] = content
	f.write(filepath.Join(f.data[i], "kafka/meta.properties"), content)
}

// zkMigrated mimics a broker migrated from ZooKeeper: V0 data directories
// (broker.id, optionally without version) and no bootstrap.checkpoint.
func (f *metadataFixture) zkMigrated(source int, versionLine string) {
	f.t.Helper()
	for i := range f.data {
		f.setMeta(i, versionLine+"cluster.id=cluster-one\nbroker.id=1\n")
	}
	require.NoError(f.t, os.Remove(filepath.Join(f.data[source], "kafka/bootstrap.checkpoint")))
}

func (f *metadataFixture) addSource(i int) {
	f.write(filepath.Join(f.data[i], "kafka/bootstrap.checkpoint"), "original-bootstrap")
	for _, name := range []string{"00000000000000000000.log", "00000000000000000000.index", "00000000000000000000.timeindex", "00000000000000000002-0000000001.checkpoint", "leader-epoch-checkpoint", "quorum-state"} {
		f.write(filepath.Join(f.data[i], "kafka/__cluster_metadata-0", name), "original-"+name+"\x00\x01\x02")
	}
}

func (f *metadataFixture) run(fault string) ([]byte, error) {
	cmd := exec.Command("bash", "-c", metadataMigrationScript)
	cmd.Env = append(append([]string{}, f.env...), "FAULT_AT="+fault, "DATA_MOUNTS="+strings.Join(f.data, ","))
	return cmd.CombinedOutput()
}

func (f *metadataFixture) assertRecovered(source int) {
	f.t.Helper()
	dest := filepath.Join(f.metadata, "kafka")
	require.FileExists(f.t, filepath.Join(dest, "meta.properties"))
	require.FileExists(f.t, filepath.Join(f.metadata, ".koperator-metadata-complete"))
	for i := range f.data {
		require.NoDirExists(f.t, filepath.Join(f.data[i], "kafka/__cluster_metadata-0"))
		data, err := os.ReadFile(filepath.Join(f.data[i], "kafka/orders-0/00000000000000000000.log"))
		require.NoError(f.t, err)
		require.Equal(f.t, "original-topic-replica", string(data))
		meta, err := os.ReadFile(filepath.Join(f.data[i], "kafka/meta.properties"))
		require.NoError(f.t, err)
		require.Equal(f.t, f.meta[i], string(meta))
	}
	backup := filepath.Join(f.data[source], ".koperator-metadata-backup-1")
	entries, err := os.ReadDir(backup)
	require.NoError(f.t, err)
	for _, entry := range entries {
		original, err := os.ReadFile(filepath.Join(backup, entry.Name()))
		require.NoError(f.t, err)
		copied, err := os.ReadFile(filepath.Join(dest, "__cluster_metadata-0", entry.Name()))
		require.NoError(f.t, err)
		require.Equal(f.t, original, copied)
	}
	sourceBootstrap, err := os.ReadFile(filepath.Join(f.data[source], "kafka/bootstrap.checkpoint"))
	if os.IsNotExist(err) {
		require.NoFileExists(f.t, filepath.Join(dest, "bootstrap.checkpoint"))
		return
	}
	require.NoError(f.t, err)
	bootstrap, err := os.ReadFile(filepath.Join(dest, "bootstrap.checkpoint"))
	require.NoError(f.t, err)
	require.Equal(f.t, sourceBootstrap, bootstrap)
}

func TestMetadataMigrationAndRecovery(t *testing.T) {
	for _, source := range []int{0, 1} {
		t.Run(fmt.Sprintf("logs%d", source+1), func(t *testing.T) {
			f := newMetadataFixture(t, source)
			output, err := f.run("")
			require.NoError(t, err, string(output))
			f.assertRecovered(source)
			output, err = f.run("")
			require.NoError(t, err, string(output))
			f.assertRecovered(source)
			// The source data disk can subsequently be removed by the normal CC lifecycle.
			f.data = []string{f.data[1-source]}
			output, err = f.run("")
			require.NoError(t, err, string(output))
		})
	}
	for _, fault := range []string{"manifest", "stage-owner", "format", "partial-format", "copy", "publish", "backup", "complete"} {
		t.Run("interrupted-"+fault, func(t *testing.T) {
			f := newMetadataFixture(t, 1)
			output, err := f.run(fault)
			require.Error(t, err, string(output))
			output, err = f.run("")
			require.NoError(t, err, string(output))
			f.assertRecovered(1)
		})
	}
	for _, tc := range []struct{ name, version string }{{"zk-migrated-v0", "version=0\n"}, {"zk-migrated-unversioned", ""}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newMetadataFixture(t, 1)
			f.zkMigrated(1, tc.version)
			output, err := f.run("")
			require.NoError(t, err, string(output))
			f.assertRecovered(1)
			output, err = f.run("")
			require.NoError(t, err, string(output))
			f.assertRecovered(1)
		})
	}
	for _, fault := range []string{"format", "copy", "publish", "backup"} {
		t.Run("zk-migrated-interrupted-"+fault, func(t *testing.T) {
			f := newMetadataFixture(t, 1)
			f.zkMigrated(1, "version=0\n")
			output, err := f.run(fault)
			require.Error(t, err, string(output))
			output, err = f.run("")
			require.NoError(t, err, string(output))
			f.assertRecovered(1)
		})
	}
	t.Run("fresh", func(t *testing.T) {
		f := newMetadataFixture(t, -1)
		output, err := f.run("")
		require.NoError(t, err, string(output))
		output, err = f.run("")
		require.NoError(t, err, string(output))
	})
}

func TestMetadataMigrationFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*metadataFixture)
	}{
		{"multiple sources", func(f *metadataFixture) { f.addSource(1) }},
		{"unowned destination", func(f *metadataFixture) { f.write(filepath.Join(f.metadata, "kafka/meta.properties"), "version=1") }},
		{"unowned staging", func(f *metadataFixture) {
			f.write(filepath.Join(f.metadata, ".koperator-metadata-stage/meta.properties"), "version=1")
		}},
		{"wrong source node", func(f *metadataFixture) {
			f.write(filepath.Join(f.data[0], "kafka/meta.properties"), "version=1\ncluster.id=cluster-one\nnode.id=2\n")
		}},
		{"wrong source cluster", func(f *metadataFixture) {
			f.write(filepath.Join(f.data[0], "kafka/meta.properties"), "version=1\ncluster.id=cluster-two\nnode.id=1\n")
		}},
		{"wrong V0 source broker", func(f *metadataFixture) {
			f.zkMigrated(0, "version=0\n")
			f.setMeta(0, "version=0\ncluster.id=cluster-one\nbroker.id=2\n")
		}},
		{"V0 source without cluster", func(f *metadataFixture) {
			f.zkMigrated(0, "version=0\n")
			f.setMeta(0, "version=0\nbroker.id=1\n")
		}},
		{"unsupported source version", func(f *metadataFixture) {
			f.setMeta(0, "version=2\ncluster.id=cluster-one\nnode.id=1\n")
		}},
		{"duplicate source version", func(f *metadataFixture) {
			f.setMeta(0, "version=1\nversion=1\ncluster.id=cluster-one\nnode.id=1\n")
		}},
		{"symlinked source bootstrap", func(f *metadataFixture) {
			bootstrap := filepath.Join(f.data[0], "kafka/bootstrap.checkpoint")
			require.NoError(f.t, os.Remove(bootstrap))
			require.NoError(f.t, os.Symlink(filepath.Join(f.root, "broker-config"), bootstrap))
		}},
		{"combined", func(f *metadataFixture) { f.write(f.config, "process.roles=broker,controller\nnode.id=1\n") }},
		{"empty source", func(f *metadataFixture) {
			require.NoError(f.t, os.RemoveAll(filepath.Join(f.data[0], "kafka/__cluster_metadata-0")))
			require.NoError(f.t, os.Mkdir(filepath.Join(f.data[0], "kafka/__cluster_metadata-0"), 0o755))
		}},
		{"lost source", func(f *metadataFixture) {
			require.NoError(f.t, os.RemoveAll(filepath.Join(f.data[0], "kafka/__cluster_metadata-0")))
		}},
		{"cloned directory ID", func(f *metadataFixture) { f.env = append(f.env, "FORMAT_ID=BBBBBBBBBBBBBBBBBBBBB0") }},
		{"missing unique directory ID", func(f *metadataFixture) { f.env = append(f.env, "FORMAT_ID=invalid") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newMetadataFixture(t, 0)
			tc.change(f)
			output, err := f.run("")
			require.Error(t, err, string(output))
			require.NoFileExists(t, filepath.Join(f.metadata, ".koperator-metadata-complete"))
			topic, err := os.ReadFile(filepath.Join(f.data[0], "kafka/orders-0/00000000000000000000.log"))
			require.NoError(t, err)
			require.Equal(t, "original-topic-replica", string(topic))
		})
	}

	t.Run("wrong published identity", func(t *testing.T) {
		f := newMetadataFixture(t, 0)
		output, err := f.run("")
		require.NoError(t, err, string(output))
		f.write(filepath.Join(f.metadata, "kafka/meta.properties"), "version=1\ncluster.id=wrong\nnode.id=1\n")
		output, err = f.run("")
		require.Error(t, err, string(output))
	})
	t.Run("existing broker without source", func(t *testing.T) {
		f := newMetadataFixture(t, -1)
		f.env = append(f.env, "ALLOW_FRESH=false")
		output, err := f.run("")
		require.Error(t, err, string(output))
	})
}

func TestMetadataStartupFormatFailurePreventsKafka(t *testing.T) {
	f := newMetadataFixture(t, -1)
	f.write(filepath.Join(f.bin, "kafka-storage.sh"), "#!/bin/bash\nexit 42\n")
	marker := filepath.Join(f.root, "kafka-started")
	f.write(filepath.Join(f.bin, "kafka-server-start.sh"), "#!/bin/bash\ntouch '"+marker+"'\n")
	cmd := exec.Command("bash", "-c", envoySidecarScript)
	cmd.Env = append(append([]string{}, f.env...), "WAIT_DIR="+filepath.Join(f.root, "wait"), "METADATA_STORAGE_ENABLED=true")
	output, err := cmd.CombinedOutput()
	require.Error(t, err, string(output))
	var exitError *exec.ExitError
	require.ErrorAs(t, err, &exitError)
	require.Equal(t, 42, exitError.ExitCode())
	require.NoFileExists(t, marker)
	require.NoFileExists(t, filepath.Join(f.root, "wait/do-not-exit-yet"))
}

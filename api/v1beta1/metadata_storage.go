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

package v1beta1

import (
	"fmt"
	"path"
	"reflect"
	"regexp"
	"strings"
)

var metadataMountPathPattern = regexp.MustCompile(`^/[a-zA-Z0-9_./-]+$`)

// ValidateMetadataStorage validates the effective (group-merged) broker configuration.
func (b *BrokerConfig) ValidateMetadataStorage(kraft bool) error {
	if b == nil || b.MetadataStorage == nil {
		return nil
	}
	if !kraft || !b.IsBrokerOnlyNode() || len(b.Roles) != 1 {
		return fmt.Errorf("metadataStorage requires a broker-only KRaft node")
	}
	m := b.MetadataStorage
	if m.PvcSpec == nil || m.EmptyDir != nil {
		return fmt.Errorf("metadataStorage requires pvcSpec and does not support emptyDir")
	}
	if m.PvcSpec.VolumeMode != nil && *m.PvcSpec.VolumeMode != "Filesystem" {
		return fmt.Errorf("metadataStorage requires a filesystem PVC")
	}
	if !metadataMountPathPattern.MatchString(m.MountPath) || path.Clean(m.MountPath) != m.MountPath || m.MountPath == "/" {
		return fmt.Errorf("metadataStorage mountPath must be a clean absolute path using letters, digits, '/', '.', '_' or '-'")
	}
	paths := []string{"/config", "/opt", "/etc", "/var/run", "/run", "/dev", "/proc", "/sys", "/usr", "/bin", "/sbin", "/lib", "/lib64", "/tmp"}
	for _, s := range b.StorageConfigs {
		paths = append(paths, s.MountPath)
	}
	for _, v := range b.VolumeMounts {
		if v.Name == "kraft-metadata" {
			return fmt.Errorf("kraft-metadata is a reserved volume name")
		}
		paths = append(paths, v.MountPath)
	}
	for _, v := range b.Volumes {
		if v.Name == "kraft-metadata" {
			return fmt.Errorf("kraft-metadata is a reserved volume name")
		}
	}
	for _, p := range paths {
		if StoragePathsOverlap(m.MountPath, p) {
			return fmt.Errorf("metadataStorage mountPath overlaps %q", p)
		}
	}
	return nil
}

// StoragePathsOverlap includes nested mounts, which can hide an existing volume.
func StoragePathsOverlap(a, b string) bool {
	a, b = path.Clean(a), path.Clean(b)
	return a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

// MetadataStorageLocationEqual allows capacity changes, but no relocation or PVC replacement.
func MetadataStorageLocationEqual(a, b *StorageConfig) bool {
	if a == nil || b == nil {
		return a == b
	}
	a, b = a.DeepCopy(), b.DeepCopy()
	if a.PvcSpec != nil && b.PvcSpec != nil {
		a.PvcSpec.Resources = b.PvcSpec.Resources
	}
	return reflect.DeepEqual(a, b)
}

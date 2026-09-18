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

import "github.com/banzaicloud/koperator/api/v1beta1"

// KeepRemovedVolume reports whether a volume (identified by its PVC mount path) that is no longer in
// the broker's desired storage config must still be kept — mounted into the broker pod, listed in its
// log.dirs, and present in Cruise Control's own capacity config — because its Cruise Control disk
// removal/rebalance has not been confirmed succeeded yet. It is the single source of truth shared by
// the broker ConfigMap (log.dirs), the broker Pod (mounted PVCs), and Cruise Control's capacity config
// so all three never disagree: keep the volume while removal is in progress (data still there, Cruise
// Control draining it), drop it only once removal is confirmed succeeded or its state has been
// cleared. On error/paused (unconfirmed success) the volume is kept to avoid data loss and allow
// retry.
func KeepRemovedVolume(volumeStates map[string]v1beta1.VolumeState, mountPath string) bool {
	volumeState, found := volumeStates[mountPath]
	if !found {
		return false
	}
	s := volumeState.CruiseControlVolumeState
	if s.IsDiskRemovalSucceeded() || s.IsDiskRebalanceSucceeded() {
		return false
	}
	return s.IsDiskRemoval() || s.IsDiskRebalance()
}

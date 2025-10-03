// Copyright © 2020 Banzai Cloud
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package resources

import (
	"encoding/json"

	"emperror.dev/errors"
	jsonpatch "github.com/evanphx/json-patch/v5"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"

	"github.com/banzaicloud/operator-tools/pkg/types"
	"github.com/banzaicloud/operator-tools/pkg/utils"
)

type GroupVersionKind struct {
	Group   string `json:"group,omitempty"`
	Version string `json:"version,omitempty"`
	Kind    string `json:"kind,omitempty"`
}

// +kubebuilder:object:generate=true
type K8SResourceOverlay struct {
	GVK       *GroupVersionKind         `json:"groupVersionKind,omitempty"`
	ObjectKey types.ObjectKey           `json:"objectKey,omitempty"`
	Patches   []K8SResourceOverlayPatch `json:"patches,omitempty"`
}

// +kubebuilder:object:generate=true
type OverlayPatchType string

const (
	ReplaceOverlayPatchType OverlayPatchType = "replace"
	DeleteOverlayPatchType  OverlayPatchType = "remove"
)

// +kubebuilder:object:generate=true
type K8SResourceOverlayPatch struct {
	Type       OverlayPatchType `json:"type,omitempty"`
	Path       *string          `json:"path,omitempty"`
	Value      *string          `json:"value,omitempty"`
	ParseValue bool             `json:"parseValue,omitempty"`
}

func PatchYAMLModifier(overlay K8SResourceOverlay, parser *ObjectParser) (ObjectModifierFunc, error) {
	if len(overlay.Patches) == 0 {
		return func(o runtime.Object) (runtime.Object, error) {
			return o, nil
		}, nil
	}

	// Build JSON Patch operations
	var patchOps []map[string]interface{}
	for _, patch := range overlay.Patches {
		op := map[string]interface{}{
			"op":   string(patch.Type),
			"path": utils.PointerToString(patch.Path),
		}

		if patch.Type == ReplaceOverlayPatchType {
			var value interface{}
			if patch.ParseValue {
				// Parse the value as YAML/JSON
				err := yaml.Unmarshal([]byte(utils.PointerToString(patch.Value)), &value)
				if err != nil {
					return nil, errors.WrapIf(err, "could not unmarshal value")
				}
			} else {
				value = utils.PointerToString(patch.Value)
			}
			op["value"] = value
		}

		patchOps = append(patchOps, op)
	}

	// Convert patch operations to JSON
	patchJSON, err := json.Marshal(patchOps)
	if err != nil {
		return nil, errors.WrapIf(err, "could not marshal patch operations")
	}

	patch, err := jsonpatch.DecodePatch(patchJSON)
	if err != nil {
		return nil, errors.WrapIf(err, "could not decode patch")
	}

	return func(o runtime.Object) (runtime.Object, error) {
		var ok bool
		var meta metav1.Object

		if overlay.GVK != nil {
			gvk := o.GetObjectKind().GroupVersionKind()
			if overlay.GVK.Group != "" && overlay.GVK.Group != gvk.Group {
				return o, nil
			}
			if overlay.GVK.Version != "" && overlay.GVK.Version != gvk.Version {
				return o, nil
			}
			if overlay.GVK.Kind != "" && overlay.GVK.Kind != gvk.Kind {
				return o, nil
			}
		}

		if meta, ok = o.(metav1.Object); !ok {
			return o, nil
		}

		if (overlay.ObjectKey.Name != "" && meta.GetName() != overlay.ObjectKey.Name) || (overlay.ObjectKey.Namespace != "" && meta.GetNamespace() != overlay.ObjectKey.Namespace) {
			return o, nil
		}

		// Marshal object to YAML
		yamlBytes, err := yaml.Marshal(o)
		if err != nil {
			return o, errors.WrapIf(err, "could not marshal runtime object")
		}

		// Convert YAML to JSON (json-patch works on JSON)
		jsonBytes, err := yaml.YAMLToJSON(yamlBytes)
		if err != nil {
			return o, errors.WrapIf(err, "could not convert YAML to JSON")
		}

		// Apply the patch
		patchedJSON, err := patch.Apply(jsonBytes)
		if err != nil {
			return o, errors.WrapIf(err, "could not apply patch")
		}

		// Convert JSON back to YAML
		patchedYAML, err := yaml.JSONToYAML(patchedJSON)
		if err != nil {
			return o, errors.WrapIf(err, "could not convert JSON to YAML")
		}

		// Parse back to K8s object
		o, err = parser.ParseYAMLToK8sObject(patchedYAML)
		if err != nil {
			return o, errors.WrapIf(err, "could not parse runtime object from yaml")
		}

		return o, nil
	}, nil
}

func ConvertGVK(gvk schema.GroupVersionKind) GroupVersionKind {
	return GroupVersionKind{
		Group:   gvk.Group,
		Version: gvk.Version,
		Kind:    gvk.Kind,
	}
}

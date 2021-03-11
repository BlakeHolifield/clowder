/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"cloud.redhat.com/clowder/v2/controllers/cloud.redhat.com/utils"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// ClowdJobInvocationSpec defines the desired state of ClowdJobInvocation
type ClowdJobInvocationSpec struct {
	// Name of the ClowdJobInvocation who owns the jobs
	AppName string `json:"appName"`

	// Jobs is the set of jobs to be run by the invocation
	Jobs []string `json:"jobs"`
}

// ClowdJobInvocationStatus defines the observed state of ClowdJobInvocation
type ClowdJobInvocationStatus struct {
	// Completed is false and updated when all jobs have either finished
	// successfully or failed past their backoff and retry values
	Completed bool `json:"completed"`
	// Jobs is a list of the job names run by Job invocation
	Jobs []string `json:"jobs"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=cji
// +kubebuilder:printcolumn:name="Completed",type="boolean",JSONPath=".status.completed"

// ClowdJobInvocation is the Schema for the jobinvocations API
type ClowdJobInvocation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClowdJobInvocationSpec   `json:"spec,omitempty"`
	Status ClowdJobInvocationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClowdJobInvocationList contains a list of ClowdJobInvocation
type ClowdJobInvocationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClowdJobInvocation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClowdJobInvocation{}, &ClowdJobInvocationList{})
}

// GetLabels returns a base set of labels relating to the ClowdJobInvocation.
func (i *ClowdJobInvocation) GetLabels() map[string]string {
	if i.Labels == nil {
		i.Labels = map[string]string{}
	}

	if _, ok := i.Labels["cji"]; !ok {
		i.Labels["cji"] = i.ObjectMeta.Name
	}

	newMap := make(map[string]string, len(i.Labels))

	for k, v := range i.Labels {
		newMap[k] = v
	}

	return newMap
}

// GetNamespacedName contructs a new namespaced name for an object from the pattern.
func (i *ClowdJobInvocation) GetNamespacedName(pattern string) types.NamespacedName {
	return types.NamespacedName{
		Namespace: i.Namespace,
		Name:      fmt.Sprintf(pattern, i.Name),
	}
}

// MakeOwnerReference defines the owner reference pointing to the ClowdJobInvocation resource.
func (i *ClowdJobInvocation) MakeOwnerReference() metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion: i.APIVersion,
		Kind:       i.Kind,
		Name:       i.ObjectMeta.Name,
		UID:        i.ObjectMeta.UID,
		Controller: utils.PointTrue(),
	}
}

// GetClowdNamespace returns the namespace of the ClowdJobInvocation object.
func (i *ClowdJobInvocation) GetClowdNamespace() string {
	return i.Namespace
}

// GetClowdName returns the name of the ClowdJobInvocation object.
func (i *ClowdJobInvocation) GetClowdName() string {
	return i.Name
}

// GetUID returns ObjectMeta.UID
func (i *ClowdJobInvocation) GetUID() types.UID {
	return i.ObjectMeta.UID
}

// SetObjectMeta sets the metadata on a ClowdApp object.
func (i *ClowdJobInvocation) SetObjectMeta(o metav1.Object, opts ...omfunc) {
	o.SetName(i.Name)
	o.SetNamespace(i.Namespace)
	o.SetLabels(i.GetLabels())
	o.SetOwnerReferences([]metav1.OwnerReference{i.MakeOwnerReference()})

	for _, opt := range opts {
		opt(o)
	}
}

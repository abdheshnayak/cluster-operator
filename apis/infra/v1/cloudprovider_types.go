package v1

import (
	"github.com/kloudlite/cluster-operator/lib/constants"
	rApi "github.com/kloudlite/cluster-operator/lib/operator"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ObjectRef struct {
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// +kubebuilder:validation:MinLength=1
	Namespace string `json:"namespace"`
}

// CloudProviderSpec defines the desired state of CloudProvider
type CloudProviderSpec struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	AccountName    string    `json:"accountName"`
	ProviderSecret ObjectRef `json:"providerSecret"`
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Provider    string `json:"provider"`
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	DisplayName string `json:"display_name"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster
//+kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.isReady",description="region"

// CloudProvider is the Schema for the cloudproviders API
type CloudProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CloudProviderSpec `json:"spec,omitempty"`
	Status rApi.Status       `json:"status,omitempty"`
}

func (cp *CloudProvider) EnsureGVK() {
	if cp != nil {
		cp.SetGroupVersionKind(GroupVersion.WithKind("CloudProvider"))
	}
}

func (in *CloudProvider) GetEnsuredAnnotations() map[string]string {
	return map[string]string{
		constants.GroupVersionKind: GroupVersion.WithKind("CloudProvider").String(),
	}
}

func (a *CloudProvider) GetEnsuredLabels() map[string]string {
	return map[string]string{
		"kloudlite.io/provider.name": a.Name,
	}
}

func (a *CloudProvider) GetStatus() *rApi.Status {
	return &a.Status
}

//+kubebuilder:object:root=true

// CloudProviderList contains a list of CloudProvider
type CloudProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloudProvider `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CloudProvider{}, &CloudProviderList{})
}

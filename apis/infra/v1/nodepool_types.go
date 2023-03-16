package v1

import (
	"fmt"

	rApi "github.com/kloudlite/cluster-operator/lib/operator"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodePoolSpec defines the desired state of NodePool
type NodePoolSpec struct {
	// +kubebuilder:validation:MinLength=1
	AccountName string `json:"accountName"`
	// +kubebuilder:validation:MinLength=1
	ClusterName string `json:"clusterName"`
	// +kubebuilder:validation:MinLength=1
	EdgeName string `json:"edgeName"`
	// +kubebuilder:validation:MinLength=1
	Provider string `json:"provider"`
	// +kubebuilder:validation:MinLength=1
	ProviderName string `json:"providerName"`
	// +kubebuilder:validation:MinLength=1
	Region string `json:"region"`
	// +kubebuilder:validation:MinLength=1
	Config string `json:"config"`
	// +kubebuilder:validation:Min: 0
	Min int `json:"min,omitempty"`
	// +kubebuilder:validation:Min: 0
	Max int `json:"max,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Account",type="string",JSONPath=".spec.accountName",description="account"
// +kubebuilder:printcolumn:name="Provider/Region",type="string",JSONPath=".metadata.annotations.provider-region",description="provider"
// +kubebuilder:printcolumn:name="Min/Max",type="string",JSONPath=".metadata.annotations.min-max",description="index of node"
// +kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.isReady",description="region"

// NodePool is the Schema for the nodepools API
type NodePool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodePoolSpec `json:"spec,omitempty"`
	Status rApi.Status  `json:"status,omitempty"`
}

func (np *NodePool) EnsureGVK() {
	if np != nil {
		np.SetGroupVersionKind(GroupVersion.WithKind("NodePool"))
	}
}

func (np *NodePool) GetEnsuredAnnotations() map[string]string {
	return map[string]string{
		"min-max":         fmt.Sprintf("%d/%d", np.Spec.Min, np.Spec.Max),
		"provider-region": fmt.Sprintf("%s/%s", np.Spec.Provider, np.Spec.Region),
	}
}

func (np *NodePool) GetEnsuredLabels() map[string]string {
	return map[string]string{
		"kloudlite.io/node-pool":     np.Name,
		"kloudlite.io/provider.name": np.Spec.ProviderName,
	}
}

func (np *NodePool) GetStatus() *rApi.Status {
	return &np.Status
}

// +kubebuilder:object:root=true

// NodePoolList contains a list of NodePool
type NodePoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodePool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodePool{}, &NodePoolList{})
}

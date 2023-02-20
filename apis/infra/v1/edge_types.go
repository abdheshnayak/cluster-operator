package v1

import (
	"fmt"

	"github.com/kloudlite/cluster-operator/lib/constants"
	rApi "github.com/kloudlite/cluster-operator/lib/operator"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Pool struct {
	// +kubebuilder:validation:MinLength=1
	Name   string `json:"name"`
	// +kubebuilder:validation:MinLength=1
	Config string `json:"config"`
	Min    int    `json:"min,omitempty"`
	Max    int    `json:"max,omitempty"`
}

// EdgeSpec defines the desired state of Edge
type EdgeSpec struct {
	// +kubebuilder:validation:MinLength=1
	AccountName  string `json:"accountName"`
	// +kubebuilder:validation:MinLength=1
	Provider     string `json:"provider,omitempty"`
	// +kubebuilder:validation:MinLength=1
	Region       string `json:"region"`
	// +kubebuilder:validation:MinLength=1
	ProviderName string `json:"providerName"`
	// +kubebuilder:validation:MinLength=1
	ClusterName  string `json:"clusterName"`

	Pools []Pool `json:"pools,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Account",type="string",JSONPath=".spec.accountName",description="account"
// +kubebuilder:printcolumn:name="Provider",type="string",JSONPath=".spec.provider",description="provider"
// +kubebuilder:printcolumn:name="Clsuter",type="string",JSONPath=".spec.clusterName",description="provider"
// +kubebuilder:printcolumn:name="pools",type="string",JSONPath=".metadata.annotations.node-pools-count",description="index of node"
//+kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.isReady",description="region"

// Edge is the Schema for the accountproviders API
type Edge struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EdgeSpec    `json:"spec,omitempty"`
	Status rApi.Status `json:"status,omitempty"`
}

func (a *Edge) GetEnsuredAnnotations() map[string]string {
	return map[string]string{
		constants.GroupVersionKind: GroupVersion.WithKind("Edge").String(),
		"node-pools-count":         fmt.Sprint(len(a.Spec.Pools)),
	}
}

func (a *Edge) GetEnsuredLabels() map[string]string {
	return map[string]string{
		"kloudlite.io/provider":      a.Spec.Provider,
		"kloudlite.io/account.name":  a.Spec.AccountName,
		"kloudlite.io/edge.name":     a.Name,
		"kloudlite.io/provider.name": a.Spec.ProviderName,
		"kloudlite.io/cluster.name":  a.Spec.ClusterName,
	}
}

func (a *Edge) GetStatus() *rApi.Status {
	return &a.Status
}

//+kubebuilder:object:root=true

// EdgeList contains a list of Edge
type EdgeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Edge `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Edge{}, &EdgeList{})
}

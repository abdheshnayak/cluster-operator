package v1

import (
	rApi "github.com/kloudlite/cluster-operator/lib/operator"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterSpec defines the desired state of Cluster
type ClusterSpec struct {
	// +kubebuilder:validation:MinLength=1
	AccountName  string `json:"accountName"`
	// +kubebuilder:validation:MinLength=1
	ProviderName string `json:"providerName"`
	// +kubebuilder:validation:MinLength=1
	Provider     string `json:"provider"`
	// +kubebuilder:validation:Min: 0
	Count        int    `json:"count"`
	// +kubebuilder:validation:MinLength=1
	Region       string `json:"region"`
	// +kubebuilder:validation:MinLength=1
	Config       string `json:"config"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster

// Cluster is the Schema for the clusters API
type Cluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterSpec `json:"spec,omitempty"`
	Status rApi.Status `json:"status,omitempty"`
}

func (in *Cluster) GetEnsuredAnnotations() map[string]string {
	return map[string]string{}
}

func (a *Cluster) GetEnsuredLabels() map[string]string {
	return map[string]string{
		"kloudlite.io/provider.name": a.Spec.ProviderName,
		"kloudlite.io/account.name":  a.Spec.AccountName,
	}
}

func (a *Cluster) GetStatus() *rApi.Status {
	return &a.Status
}

//+kubebuilder:object:root=true

// ClusterList contains a list of Cluster
type ClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Cluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Cluster{}, &ClusterList{})
}

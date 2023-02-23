package v1

import (
	rApi "github.com/kloudlite/cluster-operator/lib/operator"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MasterNodeSpec defines the desired state of MasterNode
type MasterNodeSpec struct {
	AccountName string `json:"accountName"`
	ClusterName string `json:"clusterName"`
	// MysqlURI     string `json:"mysqlURI"`
	ProviderName string `json:"providerName"`
	Provider     string `json:"provider"`
	Config       string `json:"config"`
	Region       string `json:"region"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster
//+kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.isReady",description="isReady"

// MasterNode is the Schema for the masternodes API
type MasterNode struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MasterNodeSpec `json:"spec,omitempty"`
	Status rApi.Status    `json:"status,omitempty"`
}

func (in *MasterNode) GetEnsuredAnnotations() map[string]string {
	return map[string]string{}
}

func (a *MasterNode) GetEnsuredLabels() map[string]string {
	return map[string]string{
		"kloudlite.io/cluster.name":  a.Spec.ClusterName,
		"kloudlite.io/provider.name": a.Spec.ProviderName,
		"kloudlite.io/account.name":  a.Spec.AccountName,
	}
}

func (a *MasterNode) GetStatus() *rApi.Status {
	return &a.Status
}

//+kubebuilder:object:root=true

// MasterNodeList contains a list of MasterNode
type MasterNodeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MasterNode `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MasterNode{}, &MasterNodeList{})
}

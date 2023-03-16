package v1

import (
	"encoding/json"
	"fmt"

	rApi "github.com/kloudlite/cluster-operator/lib/operator"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MasterNodeSpec defines the desired state of MasterNode
type MasterNodeSpec struct {
	// +kubebuilder:validation:MinLength=1
	AccountName string `json:"accountName"`
	// +kubebuilder:validation:MinLength=1
	ClusterName string `json:"clusterName"`
	// +kubebuilder:validation:MinLength=1
	ProviderName string `json:"providerName"`
	// +kubebuilder:validation:MinLength=1
	Provider     string `json:"provider"`
	// +kubebuilder:validation:MinLength=1
	Config       string `json:"config"`
	// +kubebuilder:validation:MinLength=1
	Region       string `json:"region"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Account",type="string",JSONPath=".spec.accountName",description="account"
// +kubebuilder:printcolumn:name="Instance",type="string",JSONPath=".metadata.annotations.instanceType",description="provider"
// +kubebuilder:printcolumn:JSONPath=".metadata.creationTimestamp",name=Age,type=date
//+kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.isReady",description="isReady"

// MasterNode is the Schema for the masternodes API
type MasterNode struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MasterNodeSpec `json:"spec,omitempty"`
	Status rApi.Status    `json:"status,omitempty"`
}

func (mn *MasterNode) EnsureGVK() {
	if mn != nil {
		mn.SetGroupVersionKind(GroupVersion.WithKind("MasterNode"))
	}
}

func (in *MasterNode) GetEnsuredAnnotations() map[string]string {
	instance := ""
	var kv map[string]string
	json.Unmarshal([]byte(in.Spec.Config), &kv)
	switch in.Spec.Provider {
	case "aws":
		instance = kv["instanceType"]
	case "do":
		instance = kv["size"]
	}

	return map[string]string{
		"instanceType": fmt.Sprintf("%s/%s%s", in.Spec.Provider, in.Spec.Region, func() string {

			if instance != "" {
				return "/" + instance
			}
			return instance
		}()),
	}

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

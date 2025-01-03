package v1

import (
	"encoding/json"
	"fmt"

	"github.com/kloudlite/cluster-operator/lib/constants"
	rApi "github.com/kloudlite/cluster-operator/lib/operator"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WorkerNodeSpec defines the desired state of WorkerNode
type WorkerNodeSpec struct {
	Stateful     bool   `json:"stateful,omitempty"`
	// +kubebuilder:validation:MinLength=1
	ClusterName  string `json:"clusterName"`
	// +kubebuilder:validation:MinLength=1
	AccountName  string `json:"accountName"`
	// +kubebuilder:validation:MinLength=1
	Region       string `json:"region"`
	// +kubebuilder:validation:MinLength=1
	EdgeName     string `json:"edgeName"`
	// +kubebuilder:validation:MinLength=1
	Provider     string `json:"provider"`
	// +kubebuilder:validation:MinLength=1
	ProviderName string `json:"providerName"`
	// +kubebuilder:validation:MinLength=1
	Config       string `json:"config"`
	// +kubebuilder:validation:MinLength=1
	Pool         string `json:"pool"`
	// +kubebuilder:default=0
	Index int `json:"nodeIndex,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Index",type="integer",JSONPath=".spec.nodeIndex",description="index of node"
// +kubebuilder:printcolumn:name="Account",type="string",JSONPath=".spec.accountName",description="account"
// +kubebuilder:printcolumn:name="Instance",type="string",JSONPath=".metadata.annotations.instanceType",description="provider"
// +kubebuilder:printcolumn:JSONPath=".metadata.creationTimestamp",name=Age,type=date
//+kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.isReady",description="region"

// WorkerNode is the Schema for the workernodes API
type WorkerNode struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkerNodeSpec `json:"spec,omitempty"`
	Status rApi.Status    `json:"status,omitempty"`
}

func (wn *WorkerNode) EnsureGVK() {
	if wn != nil {
		wn.SetGroupVersionKind(GroupVersion.WithKind("WorkerNode"))
	}
}

func (a *WorkerNode) GetEnsuredAnnotations() map[string]string {
	instance := ""
	var kv map[string]string
	json.Unmarshal([]byte(a.Spec.Config), &kv)
	switch a.Spec.Provider {
	case "aws":
		instance = kv["instanceType"]
	case "do":
		instance = kv["size"]
	}

	return map[string]string{
		"instanceType": fmt.Sprintf("%s/%s%s", a.Spec.Provider, a.Spec.Region, func() string {

			if instance != "" {
				return "/" + instance
			}
			return instance
		}()),
	}
}

func (a *WorkerNode) GetEnsuredLabels() map[string]string {
	return map[string]string{
		"kloudlite.io/account-node.name": a.Name,
		constants.AccountNameKey:         a.Spec.AccountName,
		"kloudlite.io/region":            a.Spec.EdgeName,
		constants.ClusterNameKey:         a.Spec.ClusterName,
		constants.NodePoolKey:            a.Spec.Pool,
		constants.NodeIndex:              fmt.Sprintf("%d", a.Spec.Index),
		"kloudlite.io/provider.name":     a.Spec.ProviderName,
	}
}

func (a *WorkerNode) GetStatus() *rApi.Status {
	return &a.Status
}

//+kubebuilder:object:root=true

// WorkerNodeList contains a list of WorkerNode
type WorkerNodeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WorkerNode `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WorkerNode{}, &WorkerNodeList{})
}

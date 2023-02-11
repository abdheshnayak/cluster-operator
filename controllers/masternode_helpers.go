package controllers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	cmgrv1 "github.com/kloudlite/cluster-operator/api/v1"
	"github.com/kloudlite/cluster-operator/lib/constants"
	"github.com/kloudlite/cluster-operator/lib/functions"
	rApi "github.com/kloudlite/cluster-operator/lib/operator"
	"github.com/kloudlite/cluster-operator/lib/templates"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type doSpec struct {
	Provider doProvider `yaml:"provider" json:"provider"`
	Node     doNode     `yaml:"node" json:"node"`
}

type awsSpec struct {
	Provider awsProvider `yaml:"provider" json:"provider"`
	Node     awsNode     `yaml:"node" json:"node"`
}

type doProvider struct {
	ApiToken  string `yaml:"apiToken" json:"apiToken"`
	AccountId string `yaml:"accountId" json:"accountId"`
}

type awsProvider struct {
	AccessKey    string `yaml:"accessKey" json:"accessKey"`
	AccessSecret string `yaml:"accessSecret" json:"accessSecret"`
	AccountId    string `yaml:"accountId" json:"accountId"`
}

type doNode struct {
	Region  string `yaml:"region" json:"region"`
	Size    string `yaml:"size" json:"size"`
	NodeId  string `yaml:"nodeId,omitempty" json:"nodeId,omitempty"`
	ImageId string `yaml:"imageId" json:"imageId"`
}

type awsNode struct {
	NodeId       string `yaml:"nodeId,omitempty" json:"nodeId,omitempty"`
	Region       string `yaml:"region" json:"region"`
	InstanceType string `yaml:"instanceType" json:"instanceType"`
	VPC          string `yaml:"vpc,omitempty" json:"vpc,omitempty"`
}

type doConfig struct {
	Version  string `yaml:"version" json:"version"`
	Action   string `yaml:"action" json:"action"`
	Provider string `yaml:"provider" json:"provider"`
	Spec     doSpec `yaml:"spec" json:"spec"`
}

type awsConfig struct {
	Version  string  `yaml:"version" json:"version"`
	Action   string  `yaml:"action" json:"action"`
	Provider string  `yaml:"provider" json:"provider"`
	Spec     awsSpec `yaml:"spec" json:"spec"`
}

type KLConfValues struct {
	StorePath   string `yaml:"storePath" json:"storePath"`
	TfTemplates string `yaml:"tfTemplatesPath" json:"tfTemplatesPath"`
	Secrets     string `yaml:"secrets" json:"secrets"`
	SSHPath     string `yaml:"sshPath" json:"sshPath"`
}

type KLConf struct {
	Version string       `yaml:"version" json:"version"`
	Values  KLConfValues `yaml:"spec" json:"spec"`
}

type KubeConfigType struct {
	APIVersion string `yaml:"apiVersion"`
	Clusters   []struct {
		Cluster struct {
			CertificateAuthorityData string `yaml:"certificate-authority-data"`
			Server                   string `yaml:"server"`
		} `yaml:"cluster"`
		Name string `yaml:"name"`
	} `yaml:"clusters"`
	Contexts []struct {
		Context struct {
			Cluster string `yaml:"cluster"`
			User    string `yaml:"user"`
		} `yaml:"context"`
		Name string `yaml:"name"`
	} `yaml:"contexts"`
	CurrentContext string `yaml:"current-context"`
	Kind           string `yaml:"kind"`
	Preferences    struct {
	} `yaml:"preferences"`
	Users []struct {
		Name string `yaml:"name"`
		User struct {
			ClientCertificateData string `yaml:"client-certificate-data"`
			ClientKeyData         string `yaml:"client-key-data"`
		} `yaml:"user"`
	} `yaml:"users"`
}

func (r *MasterNodeReconciler) getJobCrd(req *rApi.Request[*cmgrv1.MasterNode], create bool) ([]byte, error) {

	ctx, obj := req.Context(), req.Object

	providerSec, err := rApi.Get(
		ctx, r.Client, types.NamespacedName{
			Name:      obj.Spec.ProviderName,
			Namespace: constants.MainNs,
		},
		&corev1.Secret{},
	)

	if err != nil {
		return nil, err
	}

	klConfig := KLConf{
		Version: "v1",
		Values: KLConfValues{
			StorePath:   r.Env.StorePath,
			TfTemplates: r.Env.TFTemplatesPath,
			SSHPath:     r.Env.SSHPath,
		},
	}

	klConfigJsonBytes, err := yaml.Marshal(klConfig)
	if err != nil {
		return nil, err
	}

	var config string
	switch obj.Spec.Provider {
	case "aws":

		accessKey, ok := providerSec.Data["accessKey"]
		if !ok {
			return nil, fmt.Errorf("AccessKey not provided in provider secret")
		}

		accessSecret, ok := providerSec.Data["accessSecret"]
		if !ok {
			return nil, fmt.Errorf("AccessSecret not provided in provider secret")
		}

		var awsNodeConfig awsNode
		if err = json.Unmarshal(
			[]byte(req.Object.Spec.Config),
			&awsNodeConfig,
		); err != nil {
			return nil, err
		}

		nodeConfig := awsConfig{
			Version: "v1",
			Action: func() string {
				if create {
					return "create"
				}

				return "delete"
			}(),
			Provider: "aws",
			Spec: awsSpec{
				Provider: awsProvider{
					AccessKey:    string(accessKey),
					AccessSecret: string(accessSecret),
					AccountId:    obj.Spec.AccountName,
				},
				Node: awsNode{
					NodeId:       mNode(obj.Name),
					Region:       obj.Spec.Region,
					InstanceType: awsNodeConfig.InstanceType,
					VPC:          awsNodeConfig.VPC,
				},
			},
		}

		cYaml, e := yaml.Marshal(nodeConfig)
		if e != nil {
			return nil, e
		}
		config = base64.StdEncoding.EncodeToString(cYaml)

	case "do":

		apiToken, ok := providerSec.Data["apiToken"]
		if !ok {
			return nil, fmt.Errorf("apiToken not provided in provider secret")
		}

		var doNodeConfig doNode
		if e := json.Unmarshal(
			[]byte(req.Object.Spec.Config),
			&doNodeConfig,
		); e != nil {
			return nil, e
		}

		nodeConfig := doConfig{
			Version: "v1",
			Action: func() string {
				if create {
					return "create"
				}

				return "delete"
			}(),
			Provider: req.Object.Spec.Provider,
			Spec: doSpec{
				Provider: doProvider{
					ApiToken:  string(apiToken),
					AccountId: req.Object.Spec.AccountName,
				},
				Node: doNode{
					NodeId:  mNode(obj.Name),
					Region:  req.Object.Spec.Region,
					Size:    doNodeConfig.Size,
					ImageId: doNodeConfig.ImageId,
				},
			},
		}

		cYaml, e := yaml.Marshal(nodeConfig)
		if e != nil {
			return nil, e
		}
		config = base64.StdEncoding.EncodeToString(cYaml)

	default:
		return nil, fmt.Errorf("unknown provider %s found", obj.Spec.Provider)
	}

	jobOut, err := templates.Parse(templates.NodeJob, map[string]any{
		"name": func() string {
			if create {
				return fmt.Sprintf("create-node-%s", obj.Name)
			}
			return fmt.Sprintf("delete-node-%s", obj.Name)
		}(),
		"namespace":  constants.MainNs,
		"nodeConfig": config,
		"klConfig":   base64.StdEncoding.EncodeToString(klConfigJsonBytes),
		"provider":   obj.Spec.Provider,
		"ownerRefs":  []metav1.OwnerReference{functions.AsOwner(obj, true)},
	})

	if err != nil {
		return nil, err
	}

	return jobOut, nil
}

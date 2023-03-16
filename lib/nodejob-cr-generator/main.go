package nodejobcrgen

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/kloudlite/cluster-operator/lib/constants"
	rApi "github.com/kloudlite/cluster-operator/lib/operator"
	"github.com/kloudlite/cluster-operator/lib/templates"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetJobCrd(ctx context.Context, client client.Client, obj JobCrdSpecs, create bool) ([]byte, error) {

	providerSec, err := rApi.Get(
		ctx, client, types.NamespacedName{
			Name:      obj.ProviderName,
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
			StorePath:   obj.JobStorePath,
			TfTemplates: obj.JobTFTemplatesPath,
			SSHPath:     obj.JobSSHPath,
		},
	}

	klConfigJsonBytes, err := yaml.Marshal(klConfig)
	if err != nil {
		return nil, err
	}

	var config string
	switch obj.Provider {
	case "aws":

		accessKey, ok := providerSec.Data["accessKey"]
		if !ok {
			return nil, fmt.Errorf("AccessKey not provided in provider secret")
		}

		accessSecret, ok := providerSec.Data["accessSecret"]
		if !ok {
			return nil, fmt.Errorf("AccessSecret not provided in provider secret")
		}

		var awsNodeConfig AwsNode
		if err = json.Unmarshal(
			[]byte(obj.Config),
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
					AccountName:  obj.AccountName,
				},
				Node: AwsNode{
					NodeId:       obj.NodeName,
					Region:       obj.Region,
					InstanceType: awsNodeConfig.InstanceType,
					ImageId:      awsNodeConfig.ImageId,
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

		var doNodeConfig DoNode
		if e := json.Unmarshal(
			[]byte(obj.Config),
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
			Provider: obj.Provider,
			Spec: doSpec{
				Provider: doProvider{
					ApiToken:    string(apiToken),
					AccountName: obj.AccountName,
				},
				Node: DoNode{
					NodeId:  obj.NodeName,
					Region:  obj.Region,
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
		return nil, fmt.Errorf("unknown provider %s found", obj.Provider)
	}

	jobOut, err := templates.Parse(templates.NodeJob, map[string]any{
		"name": func() string {
			if create {
				return fmt.Sprintf("create-node-%s", obj.Name)
			}
			return fmt.Sprintf("delete-node-%s", obj.Name)
		}(),
		"namespace":     constants.MainNs,
		"nodeConfig":    config,
		"klConfig":      base64.StdEncoding.EncodeToString(klConfigJsonBytes),
		"provider":      obj.Provider,
		"sshSecretName": fmt.Sprintf("ssh-cluster-%s", obj.ClusterName),
		"ownerRefs":     obj.Owners,
	})

	if err != nil {
		return nil, err
	}

	return jobOut, nil
}

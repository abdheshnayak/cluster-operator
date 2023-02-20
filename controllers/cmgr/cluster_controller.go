package cmgr

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	cmgrv1 "github.com/kloudlite/cluster-operator/apis/cmgr/v1"
	"github.com/kloudlite/cluster-operator/env"
	"github.com/kloudlite/cluster-operator/lib/constants"
	fn "github.com/kloudlite/cluster-operator/lib/functions"
	"github.com/kloudlite/cluster-operator/lib/logging"
	rApi "github.com/kloudlite/cluster-operator/lib/operator"
	stepResult "github.com/kloudlite/cluster-operator/lib/operator/step-result"
	"github.com/kloudlite/cluster-operator/lib/templates"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiLabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

/*
# steps
1. check for the provider.
2. check if requirement satified.
3. if req not satified full fill it.

# steps initializing the cluster
4. create a master node.
5. create mysql db and expose it to the external world.
6. install k3s.
7. fetch k3s config and save k3s config to secret.
8. attach masters according to requirement.
*/

// ClusterReconciler reconciles a Cluster object
type ClusterReconciler struct {
	Env *env.Env
	client.Client
	Scheme *runtime.Scheme
	logger logging.Logger
	Name   string
}

func (r *ClusterReconciler) GetName() string {
	return r.Name
}

const (
	ReconcilationPeriod time.Duration = 30
)

const (
	ClusterReady string = "cluster-ready"
	// ProviderAvailable   string = "provider-available"
	// KubeConfigAvailable string = "kubeconfig-available"
)

const (
	mySqlServiceNS string = "kl-core"
)

//+kubebuilder:rbac:groups=cmgr.kloudlite.io,resources=clusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cmgr.kloudlite.io,resources=clusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cmgr.kloudlite.io,resources=clusters/finalizers,verbs=update

func (r *ClusterReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	req, err := rApi.NewRequest(rApi.NewReconcilerCtx(ctx, r.logger), r.Client, request.NamespacedName, &cmgrv1.Cluster{})
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if step := req.EnsureChecks(ClusterReady); !step.ShouldProceed() {
		return step.ReconcilerResponse()
	}

	if req.Object.GetDeletionTimestamp() != nil {
		if x := r.finalize(req); !x.ShouldProceed() {
			return x.ReconcilerResponse()
		}
		return ctrl.Result{}, nil
	}

	req.Logger.Infof("NEW RECONCILATION")

	if step := req.ClearStatusIfAnnotated(); !step.ShouldProceed() {
		return step.ReconcilerResponse()
	}

	if step := req.RestartIfAnnotated(); !step.ShouldProceed() {
		return step.ReconcilerResponse()
	}

	if step := req.EnsureLabelsAndAnnotations(); !step.ShouldProceed() {
		return step.ReconcilerResponse()
	}

	if step := req.EnsureFinalizers(constants.ForegroundFinalizer, constants.CommonFinalizer); !step.ShouldProceed() {
		return step.ReconcilerResponse()
	}

	if step := r.fetchRequired(req); !step.ShouldProceed() {
		return step.ReconcilerResponse()
	}

	if step := r.ReconcileCluster(req); !step.ShouldProceed() {
		return step.ReconcilerResponse()
	}

	// fmt.Printf("here are checks: %+v\n", req.Object.Status.Checks)

	req.Object.Status.IsReady = true
	req.Logger.Infof("RECONCILATION COMPLETE")
	return ctrl.Result{RequeueAfter: ReconcilationPeriod * time.Second}, r.Status().Update(ctx, req.Object)
}

func (r *ClusterReconciler) fetchRequired(req *rApi.Request[*cmgrv1.Cluster]) stepResult.Result {

	ctx, obj := req.Context(), req.Object

	// fetching providerSec
	if err := func() error {
		providerSec, err := rApi.Get(
			ctx, r.Client, types.NamespacedName{
				Name:      obj.Spec.ProviderName,
				Namespace: constants.MainNs,
			},
			&corev1.Secret{},
		)

		if err != nil {
			return err
		}

		rApi.SetLocal(req, "provider-secret", providerSec)

		return nil
	}(); err != nil {
		r.logger.Warnf(err.Error())
	}

	// fetching kubeConfig
	if err := func() error {
		kubeConfig, err := rApi.Get(
			ctx, r.Client, types.NamespacedName{
				Name:      fmt.Sprintf("cluster-kubeconfig-%s", obj.Name),
				Namespace: constants.MainNs,
			},
			&corev1.Secret{},
		)

		if err != nil {
			return err
		}

		rApi.SetLocal(req, "kubeconfig-sec", kubeConfig)

		return nil
	}(); err != nil {
		r.logger.Debugf(err.Error())
	}

	// check.Status = true
	// if check != checks[ClusterReady] {
	// 	checks[ClusterReady] = check
	// 	return req.UpdateStatus()
	// }

	return req.Next()

}

func (r *ClusterReconciler) ReconcileCluster(req *rApi.Request[*cmgrv1.Cluster]) stepResult.Result {
	ctx, obj, checks := req.Context(), req.Object, req.Object.Status.Checks
	check := rApi.Check{Generation: obj.Generation}
	failed := func(err error) stepResult.Result {
		fmt.Println("here")
		r.logger.Errorf(err,"")
		return req.CheckFailed(ClusterReady, check, err.Error())
	}
	/*
		# actions needs to be perfomed here
			check if kubeconfig present
				- if not present check for the provider
					- if provider present create cluster according to requirement
					- if not present throw error
				- if present fetch all nodes and check if matches the requirement
					- if matches update the cpu count to status
					- if not matches update the current configurations
	*/
	kubeConfigSec, ok := rApi.GetLocal[*corev1.Secret](req, "kubeconfig-sec")

	if ok {
		// fetch nodes of the cluster
		out, err := fn.KubectlWithConfig("get nodes -ojson", kubeConfigSec.Data["kubeconfig"])
		if err != nil {
			return failed(err)
		}

		var nodeList corev1.NodeList

		if err := json.Unmarshal(out, &nodeList); err != nil {
			return failed(err)
		}
		totalNodes, err := templates.Parse(templates.TotalNodes, nodeList)

		if err != nil {
			return failed(err)
		}

		obj.Status.DisplayVars.Set("cluster-nodes", string(totalNodes))
	}

	if err := func() error {
		if ok {
			return nil
		}
		// create cluster
		/*
			create mysqldb
				- check if not created create
				- if created use that
			create node
			install k3s with mysql config
		*/

		_, ok := rApi.GetLocal[*corev1.Secret](req, "provider-secret")
		if !ok {
			return fmt.Errorf("provider secret %s not found to perfom any action", obj.Spec.ProviderName)
		}

		dbName := fmt.Sprintf("cluster-%s", obj.Name)

		type ManagedResource struct {
			Status struct {
				IsReady bool `yaml:"isReady" json:"isReady"`
			} `yaml:"status" json:"status"`
		}

		// fetch mysqldb if not present create one
		mRes, err := fn.ExecCmd(
			fmt.Sprintf("kubectl get managedresources -n=%s %s -ojson", mySqlServiceNS, dbName), "", true,
		)
		if err != nil {
			// needs to create db
			res, err := templates.Parse(templates.SqlCrd, map[string]any{
				"name":      dbName,
				"msvc-name": r.Env.MySqlServiceName,
				"namespace": mySqlServiceNS,
			})
			if err != nil {
				return err
			}

			if _, err = fn.KubectlApplyExec(res); err != nil {
				return err
			}
			return fmt.Errorf("managed resource creating")
		}

		var mResource ManagedResource

		if err := json.Unmarshal(mRes, &mResource); err != nil {
			return err
		}

		if !mResource.Status.IsReady {
			return fmt.Errorf("database is not ready. waiting for it to be ready")
		}

		// TODO
		// // fetchUri

		mysqlSecret, err := rApi.Get(
			ctx, r.Client, types.NamespacedName{
				Name:      fmt.Sprintf("mres-%s", dbName),
				Namespace: constants.MainNs, // TODO
			},
			&corev1.Secret{},
		)
		if err != nil {
			return err
		}
		// // we got uri for the node creation
		uri, ok := mysqlSecret.Data["DSN"]
		if !ok {
			return fmt.Errorf("can't get dsn of db")
		}

		// uri := "mysql://49b0033e88e45096fb4f33a30ee0c359:boe-ffRaO02YdoZLCtj1FFBLxmx6CPAfz0BAiVlt@tcp(clusterdb.kloudlite.io:30001)/cluster_test_01"

		// check masters and match it to requirement
		var masterNodes cmgrv1.MasterNodeList
		if err := r.Client.List(
			ctx, &masterNodes, &client.ListOptions{
				LabelSelector: apiLabels.SelectorFromValidatedSet(
					map[string]string{
						"kloudlite.io/cluster.name": obj.Name,
					},
				),
			},
		); err != nil {
			return err
		}

		if len(masterNodes.Items) == 0 && obj.Spec.Count > 0 {
			// no nodes created yet create at least one node
			if err := r.Client.Create(ctx, &cmgrv1.MasterNode{
				ObjectMeta: metav1.ObjectMeta{Name: string(uuid.NewUUID()),
					OwnerReferences: []metav1.OwnerReference{
						fn.AsOwner(obj, true),
					},
				},
				Spec: cmgrv1.MasterNodeSpec{
					ClusterName:  obj.Name,
					MysqlURI:     string(uri),
					ProviderName: obj.Spec.ProviderName,
					Provider:     obj.Spec.Provider,
					Config:       obj.Spec.Config,
					AccountName:  obj.Spec.AccountName,
					Region:       obj.Spec.Region,
				},
			}); err != nil {
				return err
			}
			return fmt.Errorf("no master node created yet, so creating one")
		}

		if len(masterNodes.Items) > obj.Spec.Count {
			// created more nodes delete one
			sort.Slice(masterNodes.Items, func(i, j int) bool {
				return masterNodes.Items[i].Name < masterNodes.Items[j].Name
			})

			if err := r.Client.Delete(ctx, &cmgrv1.MasterNode{
				ObjectMeta: metav1.ObjectMeta{
					Name: masterNodes.Items[len(masterNodes.Items)-1].Name,
				},
			}); err != nil {
				return err
			}
			return fmt.Errorf("master node count was greater than requirement, so deleting one")
		}

		if len(masterNodes.Items) < obj.Spec.Count {
			// needs to create more node
			if err := r.Client.Create(ctx, &cmgrv1.MasterNode{
				ObjectMeta: metav1.ObjectMeta{
					Name: string(uuid.NewUUID()),
				},
				Spec: cmgrv1.MasterNodeSpec{
					ClusterName:  obj.Name,
					MysqlURI:     string(uri),
					ProviderName: obj.Spec.ProviderName,
					Provider:     obj.Spec.Provider,
					Config:       obj.Spec.Config,
				},
			}); err != nil {
				return err
			}

			return fmt.Errorf("master node count was less than requirement, so creating one")
		}

		// out, err := templates.Parse(templates.MasterNodes, masterNodes)
		// if err != nil {
		// 	return err
		// }
		//
		// obj.Status.DisplayVars.Set("master-nodes", string(out))
		return nil
	}(); err != nil {
		return failed(err)
	}

	check.Status = true
	if check != checks[ClusterReady] {
		checks[ClusterReady] = check
		req.UpdateStatus()
	}

	return req.Next()
}

func (r *ClusterReconciler) finalize(req *rApi.Request[*cmgrv1.Cluster]) stepResult.Result {
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterReconciler) SetupWithManager(mgr ctrl.Manager, logger logging.Logger) error {
	r.Client = mgr.GetClient()
	r.Scheme = mgr.GetScheme()
	r.logger = logger.WithName(r.Name)

	return ctrl.NewControllerManagedBy(mgr).
		For(&cmgrv1.Cluster{}).
		Owns(&cmgrv1.MasterNode{}).
		Complete(r)
}

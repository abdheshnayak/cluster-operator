package infra

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiLabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	infrav1 "github.com/kloudlite/cluster-operator/apis/infra/v1"
	"github.com/kloudlite/cluster-operator/lib/constants"
	"github.com/kloudlite/cluster-operator/lib/functions"
	"github.com/kloudlite/cluster-operator/lib/kresource"
	"github.com/kloudlite/cluster-operator/lib/logging"
	nodejobcrgen "github.com/kloudlite/cluster-operator/lib/nodejob-cr-generator"
	rApi "github.com/kloudlite/cluster-operator/lib/operator"
	stepResult "github.com/kloudlite/cluster-operator/lib/operator/step-result"
	"github.com/kloudlite/cluster-operator/lib/rcalculate"
)

type NodePoolReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	logger logging.Logger
	Name   string
}

func (r *NodePoolReconciler) GetName() string {
	return r.Name
}

const (
	ReconcilationPeriod time.Duration = 30
)

const (
	WorkerNodesReady   string = "worker-nodes-ready"
	WorkerNodesDeleted string = "worker-nodes-deleted"
)

// +kubebuilder:rbac:groups=infra.kloudlite.io,resources=nodepools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infra.kloudlite.io,resources=nodepools/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infra.kloudlite.io,resources=nodepools/finalizers,verbs=update

func (r *NodePoolReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	req, err := rApi.NewRequest(rApi.NewReconcilerCtx(ctx, r.logger), r.Client, request.NamespacedName, &infrav1.NodePool{})
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if step := req.EnsureChecks(WorkerNodesReady, WorkerNodesDeleted); !step.ShouldProceed() {
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

	if step := r.reconWorkerNodes(req); !step.ShouldProceed() {
		return step.ReconcilerResponse()
	}

	req.Object.Status.IsReady = true
	req.Logger.Infof("RECONCILATION COMPLETE")
	return ctrl.Result{RequeueAfter: ReconcilationPeriod * time.Second}, r.Status().Update(ctx, req.Object)
}

func (r *NodePoolReconciler) finalize(req *rApi.Request[*infrav1.NodePool]) stepResult.Result {
	// return req.Finalize()
	ctx, obj := req.Context(), req.Object

	check := rApi.Check{Generation: obj.Generation}

	var workerNodes infrav1.WorkerNodeList
	if err := r.Client.List(
		ctx, &workerNodes, &client.ListOptions{
			LabelSelector: apiLabels.SelectorFromValidatedSet(
				map[string]string{"kloudlite.io/node-pool": obj.Name},
			),
		},
	); err != nil {
		if !apiErrors.IsNotFound(err) {
			return req.CheckFailed(WorkerNodesDeleted, check, err.Error())
		}
		return req.CheckFailed(WorkerNodesDeleted, check, err.Error())
	}

	if len(workerNodes.Items) != 0 {
		return req.Done()
	}

	// if len(accountNodes.Items) >= 1 {
	// 	if err := r.DeleteAllOf(
	// 		ctx, &infrav1.AccountNode{}, &client.DeleteAllOfOptions{
	// 			ListOptions: client.ListOptions{
	// 				LabelSelector: apiLabels.SelectorFromValidatedSet(
	// 					map[string]string{
	// 						constants.RegionKey: obj.Spec.EdgeRef,
	// 					},
	// 				),
	// 			},
	// 		},
	// 	); err != nil {
	// 		return req.CheckFailed(AccountNodesDeleted, check, err.Error())
	// 	}
	// 	checks[AccountNodesDeleted] = check
	// 	return req.UpdateStatus()
	// }

	// if len(accountNodes.Items) != 0 {
	// 	return req.Done()
	// }

	// TODO: (FIXED) (watch for all nodepools to be deleted, prior to releasing finalizers)
	return req.Finalize()
}

type iDetails struct {
	Aws map[string]iDetail `json:"aws" yaml:"aws,omitempty"`
	Do  map[string]iDetail `json:"do" yaml:"do,omitempty"`
}

type iDetail struct {
	Cpu    int  `json:"cpu" yaml:"cpu"`
	Memory int  `json:"memory" yaml:"memory"`
	Gpu    bool `json:"gpu" yaml:"gpu"`
}

func (r *NodePoolReconciler) fetchRequired(req *rApi.Request[*infrav1.NodePool]) stepResult.Result {
	ctx := req.Context()
	if err := func() error {
		iDetailsConfig, err := rApi.Get(ctx, r.Client, types.NamespacedName{
			Namespace: "kl-core",
			Name:      "instance-details",
		}, &corev1.ConfigMap{})
		if err != nil {
			return err
		}

		var miDetails iDetails
		if err := yaml.Unmarshal([]byte(iDetailsConfig.Data["details"]), &miDetails); err != nil {
			return err
		}

		rApi.SetLocal(req, "instance-details", miDetails)
		return nil
	}(); err != nil {
		r.logger.Warnf(err.Error())
	}
	return req.Next()
}

func (r *NodePoolReconciler) reconWorkerNodes(req *rApi.Request[*infrav1.NodePool]) stepResult.Result {
	ctx, obj, checks, logger := req.Context(), req.Object, req.Object.Status.Checks, r.logger
	check := rApi.Check{Generation: obj.Generation}
	failed := func(err error) stepResult.Result {
		return req.CheckFailed(WorkerNodesReady, check, err.Error())
	}

	var workerNodes infrav1.WorkerNodeList
	if err := r.List(
		ctx, &workerNodes, &client.ListOptions{
			LabelSelector: apiLabels.SelectorFromValidatedSet(
				apiLabels.Set{
					"kloudlite.io/node-pool": obj.Name,
				},
			),
		},
	); err != nil {
		if !apiErrors.IsNotFound(err) {
			// return req.CheckFailed(WorkerNodesReady, check, err.Error())
			return failed(err)
		}
	}

	miDetails, ok := rApi.GetLocal[iDetails](req, "instance-details")
	if !ok {
		return failed(fmt.Errorf("instance-details not found to make decision"))
	}

	getResourceSize := func(node infrav1.WorkerNode) (*rcalculate.Size, error) {
		size := rcalculate.Size{
			Memory: 0,
			Cpu:    0,
		}

		switch node.Spec.Provider {
		case "do":
			var nodeConf nodejobcrgen.DoNode
			if err := json.Unmarshal([]byte(node.Spec.Config), &nodeConf); err != nil {
				return nil, err
			}

			res, ok := miDetails.Do[nodeConf.Size]
			if !ok {
				return nil, fmt.Errorf("details of node type %s not found", nodeConf.Size)
			}

			size = rcalculate.Size{
				Memory: res.Memory,
				Cpu:    res.Cpu,
			}

		case "aws":
			var nodeConf nodejobcrgen.AwsNode
			if err := json.Unmarshal([]byte(node.Spec.Config), &nodeConf); err != nil {
				return nil, err
			}

			res, ok := miDetails.Aws[nodeConf.InstanceType]
			if !ok {
				return nil, fmt.Errorf("details of instance type %s not found", nodeConf.InstanceType)
			}

			size = rcalculate.Size{
				Memory: res.Memory,
				Cpu:    res.Cpu,
			}
		}

		return &rcalculate.Size{
			Memory: size.Memory * 1000,
			Cpu:    size.Cpu * 1000,
		}, nil
	}

	totalUsedRes, err := kresource.GetTotalPodRequest(
		map[string]string{
			constants.RegionKey: obj.Spec.EdgeName,
		}, "requests",
	)
	if err != nil {
		return req.CheckFailed(WorkerNodesReady, check, err.Error())
	}

	totalStatefulUsedRes, err := kresource.GetTotalPodRequest(
		map[string]string{
			constants.RegionKey:          obj.Spec.EdgeName,
			"kloudlite.io/stateful-node": "true",
		}, "requests",
	)
	if err != nil {
		return req.CheckFailed(WorkerNodesReady, check, err.Error())
	}

	var nodes []rcalculate.Node

	for _, n := range workerNodes.Items {

		resSize, err := getResourceSize(n)
		if err != nil {
			return failed(fmt.Errorf("error on node %s, [%s]", n.Name, err.Error()))
		}

		nodes = append(
			nodes, rcalculate.Node{
				Name:     n.Name,
				Stateful: n.Spec.Stateful,
				Size:     *resSize,
			},
		)
	}

	i := rcalculate.Input{
		MinNode:      obj.Spec.Min,
		MaxNode:      obj.Spec.Max,
		Nodes:        nodes,
		StatefulUsed: totalStatefulUsedRes.Memory,
		TotalUsed:    totalUsedRes.Memory,
		Threshold:    80,
	}
	logger.Infof("Scale-> %s", obj.Name)
	action, msg, err := i.Calculate(logger)
	if err != nil {
		return req.CheckFailed(WorkerNodesReady, check, err.Error())
	}

	logger.Infof("\n\n\n%d: %s (%s)\n\n\n", action, *msg, req.Object.Name)

	if true {
		switch action {
		case rcalculate.ADD_NODE:
			{
				if err := r.Client.Create(
					ctx, &infrav1.WorkerNode{
						ObjectMeta: metav1.ObjectMeta{
							Name: string(uuid.NewUUID()),
							Labels: apiLabels.Set{
								constants.RegionKey: obj.Spec.EdgeName,
							},
							OwnerReferences: []metav1.OwnerReference{functions.AsOwner(obj, true)},
						},
						Spec: infrav1.WorkerNodeSpec{
							ClusterName:  obj.Spec.ClusterName,
							ProviderName: obj.Spec.ProviderName,
							AccountName:  obj.Spec.AccountName,
							EdgeName:     obj.Spec.EdgeName,
							Provider:     obj.Spec.Provider,
							Config:       obj.Spec.Config,
							Region:       obj.Spec.Region,
							Pool:         obj.Name,
							Index: func() int {
								ind := len(workerNodes.Items)
								for i := 0; i < ind; i++ {
									found := false
									for _, n := range workerNodes.Items {
										if i == int(n.Spec.Index) {
											found = true
											break
										}
									}
									if !found {
										return i
									}
								}
								return ind
							}(),
						},
					},
				); err != nil {
					return req.CheckFailed(WorkerNodesReady, check, err.Error())
				}
				return req.Done()
			}

		case rcalculate.DEL_NODE:
			{
				// find last node and delete

				if len(workerNodes.Items) > 0 {

					last := 0
					for _, n := range workerNodes.Items {
						if last < n.Spec.Index {
							last = n.Spec.Index
						}
					}

					for _, n := range workerNodes.Items {
						if n.Spec.Index == last {
							if err := r.Delete(
								ctx, &infrav1.WorkerNode{
									ObjectMeta: metav1.ObjectMeta{
										Name: n.Name,
										// Namespace: n.Namespace,
									},
								},
							); err != nil {
								return req.CheckFailed(WorkerNodesReady, check, err.Error())
							}
							break
						}
					}
					return req.Done()
				}
			}
		case rcalculate.ADD_STATEFUL:
			{
				cnt := i.GetStatefulCount()
				for _, n := range workerNodes.Items {
					if n.Spec.Index == cnt {
						ctrl.CreateOrUpdate(
							ctx, r.Client, &n, func() error {
								n.Spec.Stateful = true
								return nil
							},
						)

						break
					}
				}
			}
		case 0:
			{
				logger.Infof("workerNodes in sync...")
				check.Status = true
			}
		}
	}

	check.Status = true
	if check != checks[WorkerNodesReady] {
		checks[WorkerNodesReady] = check
		return req.UpdateStatus()
	}

	return req.Next()
}

func (r *NodePoolReconciler) SetupWithManager(mgr ctrl.Manager, logger logging.Logger) error {
	r.logger = logger.WithName(r.Name)
	r.Client = mgr.GetClient()
	r.Scheme = mgr.GetScheme()

	builder := ctrl.NewControllerManagedBy(mgr).For(&infrav1.NodePool{})
	builder.Owns(&infrav1.WorkerNode{})

	watchList := []client.Object{
		&corev1.Node{},
		&corev1.Pod{},
	}

	for i := range watchList {
		builder.Watches(
			&source.Kind{Type: watchList[i]}, handler.EnqueueRequestsFromMapFunc(
				func(o client.Object) []reconcile.Request {
					l, ok := o.GetLabels()[constants.RegionKey]
					if !ok {
						return nil
					}
					return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: l}}}
				},
			),
		)
	}

	return builder.Complete(r)
}

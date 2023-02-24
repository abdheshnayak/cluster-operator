package infra

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/kloudlite/cluster-operator/lib/kubectl"
	nodejobcrgen "github.com/kloudlite/cluster-operator/lib/nodejob-cr-generator"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrav1 "github.com/kloudlite/cluster-operator/apis/infra/v1"
	"github.com/kloudlite/cluster-operator/env"
	"github.com/kloudlite/cluster-operator/lib/constants"
	fn "github.com/kloudlite/cluster-operator/lib/functions"
	"github.com/kloudlite/cluster-operator/lib/logging"
	rApi "github.com/kloudlite/cluster-operator/lib/operator"
	stepResult "github.com/kloudlite/cluster-operator/lib/operator/step-result"
	"github.com/kloudlite/cluster-operator/lib/terraform"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
)

/*
# actions needs to be performed
1. check if node created
2. if not created create
3. check if node attached
4. if not attached
5. if deletion timestamp present
	- delete node from the cluster
*/

// WorkerNodeReconciler reconciles a WorkerNode object
type WorkerNodeReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	logger logging.Logger
	Name   string
	Env    *env.Env
}

func (r *WorkerNodeReconciler) GetName() string {
	return r.Name
}

const (
	NodeCreated  string = "node-created"
	NodeAttached string = "node-attached"
	SSHReady     string = "ssh-ready"
)

//+kubebuilder:rbac:groups=infra.kloudlite.io,resources=workernodes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infra.kloudlite.io,resources=workernodes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infra.kloudlite.io,resources=workernodes/finalizers,verbs=update

func (r *WorkerNodeReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {

	req, err := rApi.NewRequest(rApi.NewReconcilerCtx(ctx, r.logger), r.Client, request.NamespacedName, &infrav1.WorkerNode{})
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if step := req.EnsureChecks(NodeCreated, NodeAttached, SSHReady); !step.ShouldProceed() {
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

	// if step := r.reconWorkerNodes(req); !step.ShouldProceed() {
	// 	return step.ReconcilerResponse()
	// }

	if step := r.fetchRequired(req); !step.ShouldProceed() {
		return step.ReconcilerResponse()
	}

	if step := r.EnsureSSH(req); !step.ShouldProceed() {
		return step.ReconcilerResponse()
	}

	if step := r.ensureNodeCreated(req); !step.ShouldProceed() {
		return step.ReconcilerResponse()
	}

	if step := r.ensureNodeAttached(req); !step.ShouldProceed() {
		return step.ReconcilerResponse()
	}

	req.Object.Status.IsReady = true
	req.Logger.Infof("RECONCILATION COMPLETE")
	return ctrl.Result{RequeueAfter: ReconcilationPeriod * time.Second}, r.Status().Update(ctx, req.Object)
}

func (r *WorkerNodeReconciler) fetchRequired(req *rApi.Request[*infrav1.WorkerNode]) stepResult.Result {
	ctx, obj := req.Context(), req.Object

	// fetching kubeConfig
	if err := func() error {

		kubeConfig, err := rApi.Get(
			ctx, r.Client, types.NamespacedName{
				Name:      fmt.Sprintf("cluster-kubeconfig-%s", obj.Spec.ClusterName),
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
		r.logger.Warnf(err.Error())
	}

	return req.Next()
}

func (r *WorkerNodeReconciler) EnsureSSH(req *rApi.Request[*infrav1.WorkerNode]) stepResult.Result {

	ctx, obj, checks := req.Context(), req.Object, req.Object.Status.Checks
	check := rApi.Check{Generation: obj.Generation}
	failed := func(err error) stepResult.Result {
		return req.CheckFailed(SSHReady, check, err.Error())
	}

	secName := fmt.Sprintf("ssh-cluster-%s", obj.Spec.ClusterName)

	_, err := rApi.Get(
		ctx, r.Client, types.NamespacedName{
			Name:      secName,
			Namespace: constants.MainNs, // TODO
		},
		&corev1.Secret{},
	)
	if err != nil {
		return failed(err)
	}

	check.Status = true
	if check != checks[SSHReady] {
		checks[SSHReady] = check
		req.UpdateStatus()
	}

	return req.Next()
}

func (r *WorkerNodeReconciler) ensureNodeCreated(req *rApi.Request[*infrav1.WorkerNode]) stepResult.Result {

	ctx, obj, checks := req.Context(), req.Object, req.Object.Status.Checks
	check := rApi.Check{Generation: obj.Generation}
	failed := func(err error) stepResult.Result {
		return req.CheckFailed(NodeCreated, check, err.Error())
	}

	_, err := terraform.GetOutputs(r.getTFPath(obj))
	if err != nil {
		// node is created // needs to check its status
		if err := r.createNode(req); err != nil {
			return failed(err)
		}

		return failed(fmt.Errorf("node scheduled to create"))
	}

	_, err = rApi.Get(
		ctx, r.Client, types.NamespacedName{
			Name:      fmt.Sprintf("create-node-%s", obj.Name),
			Namespace: constants.MainNs,
		},
		&batchv1.Job{},
	)

	if err == nil {
		if err := r.Client.Delete(ctx, &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("create-node-%s", obj.Name),
				Namespace: constants.MainNs,
			},
		}); err != nil {
			return failed(err)
		}
	}

	// TODO: ensure it is running

	check.Status = true
	if check != checks[NodeCreated] {
		checks[NodeCreated] = check
		return req.UpdateStatus()
	}
	return req.Next()
}

// ensure it's attached to the cluster

func getK8sClient(req *rApi.Request[*infrav1.WorkerNode], scheme *runtime.Scheme) (client.Client, error) {

	kubeConfigSec, ok := rApi.GetLocal[*corev1.Secret](req, "kubeconfig-sec")
	if !ok {
		return nil, fmt.Errorf("cluster config not found in secret")
	}
	kubeconfigBytes, ok := kubeConfigSec.Data["kubeconfig"]
	if !ok {
		return nil, fmt.Errorf("cluster config not found in secret")
	}
	return kubectl.NewCliFromConfigBytes(scheme, kubeconfigBytes)
}

func (r *WorkerNodeReconciler) ensureNodeAttached(req *rApi.Request[*infrav1.WorkerNode]) stepResult.Result {

	ctx, obj, checks := req.Context(), req.Object, req.Object.Status.Checks
	check := rApi.Check{Generation: obj.Generation}
	failed := func(err error) stepResult.Result {
		return req.CheckFailed(NodeAttached, check, err.Error())
	}

	clustClient, err := rApi.GetK8sClient(req, r.Scheme)
	if err != nil {
		return failed(err)
	}

	node, err := rApi.Get(ctx, clustClient, types.NamespacedName{
		Name: mNode(obj.Name),
	}, &corev1.Node{})

	if err != nil {
		if !apiErrors.IsNotFound(err) {
			return failed(err)
		}

		if err := r.attachNode(req); err != nil {
			return failed(err)
		}

		return failed(fmt.Errorf("attaching node to cluster"))

	}

	if isStateful, ok := node.Labels["kloudlite.io/stateful-node"]; !ok || isStateful != fmt.Sprint(obj.Spec.Stateful) {
		node.Labels["kloudlite.io/stateful-node"] = fmt.Sprint(obj.Spec.Stateful)
		if err = clustClient.Update(ctx, node); err != nil {
			return failed(err)
		}
	}

	check.Status = true
	if check != checks[NodeAttached] {
		checks[NodeAttached] = check
		return req.UpdateStatus()
	}
	return req.Next()
}

func (r *WorkerNodeReconciler) deleteNode(req *rApi.Request[*infrav1.WorkerNode]) error {

	ctx, obj := req.Context(), req.Object

	jobOut, err := nodejobcrgen.GetJobCrd(ctx, r.Client, nodejobcrgen.JobCrdSpecs{
		Name:               obj.Name,
		ProviderName:       obj.Spec.ProviderName,
		JobStorePath:       r.Env.JobStorePath,
		JobTFTemplatesPath: r.Env.JobTFTemplatesPath,
		JobSSHPath:         r.Env.JobSSHPath,
		Provider:           obj.Spec.Provider,
		Config:             obj.Spec.Config,
		AccountName:        obj.Spec.AccountName,
		Region:             obj.Spec.Region,
		Owners:             []metav1.OwnerReference{fn.AsOwner(obj, true)},
		ClusterName:        obj.Spec.ClusterName,
		NodeName:           mNode(obj.Name),
	}, false)
	if err != nil {
		return err
	}

	if _, err = fn.KubectlApplyExec(jobOut); err != nil {
		return err
	}

	return fmt.Errorf("node scheduled to delete")
}

func (r *WorkerNodeReconciler) finalize(req *rApi.Request[*infrav1.WorkerNode]) stepResult.Result {
	ctx, obj := req.Context(), req.Object
	failed := req.FailWithStatusError

	node, err := rApi.Get(ctx, r.Client, types.NamespacedName{
		Name: mNode(obj.Name),
	}, &corev1.Node{})

	if err == nil {

		clustClient, err := rApi.GetK8sClient(req, r.Scheme)
		if err != nil {
			return failed(err)
		}

		if err := clustClient.Delete(req.Context(), node); err != nil {
			return req.FailWithStatusError(err)
		}
		return failed(fmt.Errorf("detaching node from cluster"))
	}

	if !apiErrors.IsNotFound(err) {
		return failed(err)
	}

	if _, err = terraform.GetOutputs(r.getTFPath(obj)); err == nil {
		// delete node here
		if err := r.deleteNode(req); err != nil {
			return failed(err)
		}
	}

	return req.Finalize()
}

func mNode(name string) string {
	return fmt.Sprintf("kl-byoc-%s", name)
}

func (r *WorkerNodeReconciler) getTFPath(obj *infrav1.WorkerNode) string {
	// eg -> /path/acc_id/do/blr1/node_id/do
	// eg -> /path/acc_id/aws/ap-south-1/node_id/aws
	return path.Join(r.Env.StorePath, obj.Spec.AccountName, obj.Spec.Provider, obj.Spec.Region, mNode(obj.Name), obj.Spec.Provider)
}

func (r *WorkerNodeReconciler) createNode(req *rApi.Request[*infrav1.WorkerNode]) error {

	ctx, obj := req.Context(), req.Object

	_, err := rApi.Get(
		ctx, r.Client, types.NamespacedName{
			Name:      fmt.Sprintf("create-node-%s", obj.Name),
			Namespace: constants.MainNs,
		},
		&batchv1.Job{},
	)

	if err == nil {
		return fmt.Errorf("creation of node is in progress")
	}

	jobOut, err := nodejobcrgen.GetJobCrd(ctx, r.Client, nodejobcrgen.JobCrdSpecs{
		Name:               obj.Name,
		ProviderName:       obj.Spec.ProviderName,
		JobStorePath:       r.Env.JobStorePath,
		JobTFTemplatesPath: r.Env.JobTFTemplatesPath,
		JobSSHPath:         r.Env.JobSSHPath,
		Provider:           obj.Spec.Provider,
		Config:             obj.Spec.Config,
		AccountName:        obj.Spec.AccountName,
		Region:             obj.Spec.Region,
		Owners:             []metav1.OwnerReference{fn.AsOwner(obj, true)},
		ClusterName:        obj.Spec.ClusterName,
		NodeName:           mNode(obj.Name),
	}, true)
	if err != nil {
		return err
	}

	if _, err = fn.KubectlApplyExec(jobOut); err != nil {
		return err
	}

	return nil
}

func (r *WorkerNodeReconciler) attachNode(req *rApi.Request[*infrav1.WorkerNode]) error {
	obj := req.Object

	// get node token
	kubeConfigSec, ok := rApi.GetLocal[*corev1.Secret](req, "kubeconfig-sec")
	if !ok {
		return fmt.Errorf("cluster config not found in secret")
	}

	token := kubeConfigSec.Data["node-token"]
	serverIp := kubeConfigSec.Data["master-node-ip"]

	ip, err := terraform.GetOutput(r.getTFPath(obj), "node-ip")
	if err != nil {
		return err
	}

	labels := func() []string {
		l := []string{}
		for k, v := range map[string]string{
			"kloudlite.io/region":     obj.Spec.EdgeName,
			"kloudlite.io/node.index": fmt.Sprint(obj.Spec.Index),
		} {
			l = append(l, fmt.Sprintf("--node-label %s=%s", k, v))
		}
		l = append(l, fmt.Sprintf("--node-label %s=%s", "kloudlite.io/public-ip", string(ip)))
		return l
	}()

	cmd := fmt.Sprintf(
		"ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -i %s root@%s sudo sh /tmp/k3s-install.sh agent --token=%s --server https://%s:6443 --node-external-ip %s --node-name %s %s",
		r.Env.SSHPath,
		ip,
		strings.TrimSpace(string(token)),
		serverIp,
		ip,
		mNode(obj.Name),
		strings.Join(labels, " "),
	)

	_, err = fn.ExecCmd(cmd, "", true)
	if err != nil {
		return err
	}

	return nil
}

func (r *WorkerNodeReconciler) SetupWithManager(mgr ctrl.Manager, logger logging.Logger) error {
	r.logger = logger.WithName(r.Name)
	r.Client = mgr.GetClient()
	r.Scheme = mgr.GetScheme()

	builder := ctrl.NewControllerManagedBy(mgr).For(&infrav1.WorkerNode{})
	builder.Owns(&batchv1.Job{})

	return builder.Complete(r)
}
